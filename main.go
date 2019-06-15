package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var stickerDict map[string][]string = map[string][]string{}
var photoDict map[string][]string = map[string][]string{}

const STICKER_JSON_PATH = "stickers.json"
const PHOTO_JSON_PATH = "photos.json"

func addItem(m *tb.Message, fromDict *sync.Map, toDict map[string][]string, path string) (bool, error) {
	fileIDif, found := fromDict.Load(m.Chat.ID)
	if !found {
		return false, nil
	}
	fileID, ok := fileIDif.(string)
	if !ok {
		return false, nil
	}
	fromDict.Delete(m.Chat.ID)
	for _, word := range strings.Split(m.Text, ",") {
		word = strings.ToLower(strings.TrimSpace(word))
		if _, found = toDict[word]; !found {
			toDict[word] = []string{}
		}
		toDict[word] = append(toDict[word], fileID)
	}

	f, err := os.Create(path)
	if err != nil {
		return false, err
	}
	json.NewEncoder(f).Encode(toDict)
	return true, nil
}

func main() {
	b, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("MEMINDEX_TELEGRAM_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 2 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	f, err := os.Open(STICKER_JSON_PATH)
	if err == nil {
		json.NewDecoder(f).Decode(&stickerDict)
	}
	f, err = os.Open(PHOTO_JSON_PATH)
	if err == nil {
		json.NewDecoder(f).Decode(&photoDict)
	}

	addingStickers := &sync.Map{}
	addingPhotos := &sync.Map{}
	b.Handle(tb.OnSticker, func(m *tb.Message) {
		b.Send(m.Sender, "cool sticker, bro... send me some keywords (separated by commas)")
		addingStickers.Store(m.Chat.ID, m.Sticker.File.FileID)
	})

	b.Handle(tb.OnPhoto, func(m *tb.Message) {
		b.Send(m.Sender, "cool photo, bro... send me some keywords (separated by commas)")
		addingPhotos.Store(m.Chat.ID, m.Photo.File.FileID)
	})

	b.Handle(tb.OnText, func(m *tb.Message) {
		type fromTo struct {
			from *sync.Map
			to   map[string][]string
			path string
		}
		for _, ft := range []fromTo{
			fromTo{addingStickers, stickerDict, STICKER_JSON_PATH}, fromTo{addingPhotos, photoDict, PHOTO_JSON_PATH}} {
			added, err := addItem(m, ft.from, ft.to, ft.path)
			if err != nil {
				log.Print(err)
				return
			}
			if added {
				b.Send(m.Sender, "ok, cool")
				return
			}
		}
	})

	b.Handle(tb.OnQuery, func(q *tb.Query) {
		stickers := map[string]bool{}
		for word, fileIDs := range stickerDict {
			if strings.HasPrefix(word, strings.ToLower(q.Text)) {
				for _, fileID := range fileIDs {
					stickers[fileID] = true
				}
			}
		}

		photos := map[string]bool{}
		for word, fileIDs := range photoDict {
			if strings.HasPrefix(word, strings.ToLower(q.Text)) {
				for _, fileID := range fileIDs {
					photos[fileID] = true
				}
			}
		}

		results := make(tb.Results, 0, len(stickers)+len(photos))
		for fileID, _ := range stickers {
			res := &tb.StickerResult{Cache: fileID}
			res.SetResultID(fileID)
			results = append(results, res)
		}
		for fileID, _ := range photos {
			res := &tb.PhotoResult{Cache: fileID}
			res.SetResultID(fileID)
			results = append(results, res)
		}

		err := b.Answer(q, &tb.QueryResponse{
			Results:   results,
			CacheTime: 10,
		})

		if err != nil {
			log.Print(err)
		}
	})

	b.Start()
}
