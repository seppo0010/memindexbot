package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var dict map[string][]string = map[string][]string{}

const JSON_PATH = "data.json"

func main() {
	b, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("MEMINDEX_TELEGRAM_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 2 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	f, err := os.Open(JSON_PATH)
	if err == nil {
		json.NewDecoder(f).Decode(&dict)
	}

	addingStickers := map[int64]string{}
	b.Handle(tb.OnSticker, func(m *tb.Message) {
		b.Send(m.Sender, "cool sticker, bro... send me some keywords (separated by commas)")
		addingStickers[m.Chat.ID] = m.Sticker.File.FileID
	})

	b.Handle(tb.OnText, func(m *tb.Message) {
		if fileID, found := addingStickers[m.Chat.ID]; found {
			for _, word := range strings.Split(m.Text, ",") {
				word = strings.ToLower(strings.TrimSpace(word))
				if _, found = dict[word]; !found {
					dict[word] = []string{}
				}
				dict[word] = append(dict[word], fileID)
			}
			delete(addingStickers, m.Chat.ID)
			b.Send(m.Sender, "ok, cool")

			f, err := os.Create(JSON_PATH)
			if err != nil {
				return
			}
			json.NewEncoder(f).Encode(dict)
		}
	})

	b.Handle(tb.OnQuery, func(q *tb.Query) {
		files := map[string]bool{}
		for word, fileIDs := range dict {
			if strings.HasPrefix(word, strings.ToLower(q.Text)) {
				for _, fileID := range fileIDs {
					files[fileID] = true
				}
			}
		}

		results := make(tb.Results, 0, len(files))
		for fileID, _ := range files {
			res := &tb.StickerResult{Cache: fileID}
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
