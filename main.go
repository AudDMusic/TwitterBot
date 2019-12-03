package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type AudDResponse struct {
	Status string `json:"status"`
	Error  struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"error"`
	Result SongInfo `json:"result"`
}

type SongInfo struct {
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Album       string `json:"album"`
	ReleaseDate string `json:"release_date"`
	Label       string `json:"label"`
	Underground bool   `json:"underground"`
	TimeCode    string `json:"timecode"`
	SongLink string    `json:"song_link"`
}

func Recognize(reader io.Reader, url, Return, apiToken string) AudDResponse {
	var apiResponse []byte
	if reader == nil {
		apiResponse = RecognizeByUrl(url, Return, apiToken)
	}
	var result AudDResponse
	json.Unmarshal(apiResponse, &result)
	return result
}
func RecognizeByUrl(url, Return, apiToken string) []byte {
	params := map[string]string{"api_token": apiToken, "url": url, "return": Return}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	writer.Close()
	req, _ := http.NewRequest("POST", "https://api.audd.io/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if capture(err) {
		panic(err)
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	return respBody
}

func main(){
	var err error
	config := oauth1.NewConfig("", "")
	token := oauth1.NewToken("", "")
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := twitter.NewClient(httpClient)
	demux := twitter.NewSwitchDemux()
	demux.Tweet = func(tweet *twitter.Tweet) {
		jsonRepresentation, err := json.Marshal(tweet)
		if capture(err) {
			return
		}
		fmt.Printf("New tweet from @%s : %s\n", tweet.User.ScreenName, jsonRepresentation)
		url := ""
		result := AudDResponse{}
		foundMedia := false
		replyTo := tweet.User.ScreenName
		replyToTweet := tweet.ID
		for !foundMedia {
			if tweet.User != nil {
				if tweet.User.ScreenName == "helloAudD" {
					break
				}
			}
			if tweet.ExtendedEntities != nil {
				if len(tweet.ExtendedEntities.Media) > 0 {
					minBitrate := 0
					for _, video := range tweet.ExtendedEntities.Media[0].VideoInfo.Variants {
						if video.ContentType == "video/mp4" && (video.Bitrate < minBitrate || minBitrate == 0) {
							foundMedia = true
							url = video.URL
							minBitrate = video.Bitrate
						}
					}
				}
			}
			if tweet.InReplyToStatusID == 0 || foundMedia {
				break
			}
			tweet, _, err = client.Statuses.Show(tweet.InReplyToStatusID, nil)
			if capture(err) {
				return
			}
			jsonRepresentation, err := json.Marshal(tweet)
			if capture(err) {
				return
			}
			fmt.Printf("It's reply to tweet from @%s : %s\n", tweet.User.ScreenName, jsonRepresentation)
		}
		if url != "" {
			result = Recognize(nil, url, "timecode,song_link_nm", "")
			tweet, _, err = client.Statuses.Show(tweet.ID, &twitter.StatusShowParams{TweetMode: "extended"})
			if capture(err) {
				return
			}
			if result.Status == "success" {
				if result.Result.Title != "" {
					status := fmt.Sprintf("@%s It's %s - %s. Listen: %s [plays on %s]",
						replyTo, result.Result.Artist, result.Result.Title, result.Result.SongLink, result.Result.TimeCode)
					_, _, err = client.Statuses.Update(status, &twitter.StatusUpdateParams{InReplyToStatusID: replyToTweet})
					if capture(err) {
						return
					}
				} else {
					status := fmt.Sprintf("@%s Couldn't recognize the song from this video :(", replyTo)
					_, _, err = client.Statuses.Update(status, &twitter.StatusUpdateParams{InReplyToStatusID: replyToTweet})
					if capture(err) {
						return
					}
				}
			}
		}
	}
	demux.DM = func(dm *twitter.DirectMessage) {
		fmt.Println(dm.SenderID)
	}
	demux.Event = func(event *twitter.Event) {
		fmt.Printf("%#v\n", event)
	}
	fmt.Println("Starting Stream...")
	// FILTER
	filterParams := &twitter.StreamFilterParams{
		Track:         []string{"@helloAudD"},
		StallWarnings: twitter.Bool(true),
	}
	stream, err := client.Streams.Filter(filterParams)
	if capture(err) {
		log.Println(err)
	}
	go raven.CapturePanic(func(){ demux.HandleChan(stream.Messages) }, nil)

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	fmt.Println("Stopping Stream...")
	stream.Stop()
}


func capture(err error) bool {
	if err == nil {
		return false
	}
	/* */
	return true
}
