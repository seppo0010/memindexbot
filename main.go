package main

import (
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jpoz/gomeme"
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

func caption(f io.Reader, top, bottom string) (io.Reader, error) {
	j, err := jpeg.Decode(f)
	if err != nil {
		return nil, err
	}
	config := gomeme.NewConfig()
	config.TopText = top
	config.BottomText = bottom
	meme := &gomeme.Meme{
		Config:   config,
		Memeable: gomeme.JPEG{j},
	}
	r, w := io.Pipe()
	go func() { meme.Write(w); w.Close() }()
	return r, nil
}

func main() {
	http_port_s := os.Getenv("MEMINDEX_HTTP_PORT")
	http_port, err := strconv.Atoi(http_port_s)
	if err != nil {
		log.Fatal("MEMINDEX_HTTP_PORT is invalid")
	}
	baseURL := os.Getenv("MEMINDEX_BASE_URL")
	if baseURL == "" {
		log.Fatal("MEMINDEX_BASE_URL is required")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		log.Fatalf("Invalid URL %s: %s", baseURL, err)
	}

	b, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("MEMINDEX_TELEGRAM_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 2 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	go func() {
		basePathSegments := strings.Split(strings.TrimRight(parsedURL.Path, "/"), "/")
		baselen := len(basePathSegments)
		http.HandleFunc(parsedURL.Path, func(w http.ResponseWriter, r *http.Request) {
			segments := strings.Split(r.URL.Path, "/")
			if len(segments) < baselen+3 {
				w.WriteHeader(400)
				return
			}
			fileID := segments[baselen]
			top := segments[baselen+1]
			bottom := segments[baselen+2]

			file, err := b.FileByID(fileID)
			if err != nil {
				log.Print(err)
				w.WriteHeader(500)
				return
			}
			reader, err := b.GetFile(&file)
			if err != nil {
				log.Print(err)
				w.WriteHeader(500)
				return
			}
			defer reader.Close()
			img, err := caption(reader, top, bottom)
			if err != nil {
				log.Print(err)
				w.WriteHeader(500)
				return
			}
			b, err := ioutil.ReadAll(img)
			if err != nil {
				log.Print(err)
				w.WriteHeader(500)
				return
			}

			w.WriteHeader(200)
			w.Write(b)
		})

		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", http_port), nil))
	}()

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
		criteria := q.Text
		top, bottom := "", ""
		if strings.Contains(criteria, ",") {
			segments := strings.Split(criteria, ",")
			criteria = segments[0]
			if len(segments) > 2 {
				top = segments[1]
				bottom = segments[2]
			} else {
				bottom = segments[1]
			}
		}
		hasCaption := top != "" || bottom != ""

		stickers := map[string]bool{}
		for word, fileIDs := range stickerDict {
			if strings.HasPrefix(word, strings.ToLower(criteria)) {
				for _, fileID := range fileIDs {
					stickers[fileID] = true
				}
			}
		}

		photos := map[string]bool{}
		for word, fileIDs := range photoDict {
			if strings.HasPrefix(word, strings.ToLower(criteria)) {
				for _, fileID := range fileIDs {
					photos[fileID] = true
				}
			}
		}

		results := make(tb.Results, 0, len(stickers)+len(photos)+len(photos)*map[bool]int{false: 0, true: 1}[hasCaption])
		if hasCaption {
			for fileID, _ := range photos {
				res := &tb.PhotoResult{URL: fmt.Sprintf("%s/%s/%s/%s", strings.TrimRight(baseURL, "/"), fileID, url.PathEscape(top), url.PathEscape(bottom))}
				res.SetResultID(fileID + ",captioned")
				results = append(results, res)
			}
		}
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
