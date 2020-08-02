package main

import (
	"encoding/json"
	"fmt"
	"github.com/AudDMusic/audd-go"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/getsentry/raven-go"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

func ContainsAnyString(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
func main(){
	var err error
	config := oauth1.NewConfig("", "")
	token := oauth1.NewToken("", "")
	auddClient := audd.NewClient("")
	// place your Twitter (consumer key, consumer secret, token, token secret) and AudD (api_token) credentials above

	httpClient := config.Client(oauth1.NoContext, token)
	client := twitter.NewClient(httpClient)
	demux := twitter.NewSwitchDemux()
	demux.Tweet = func(tweet *twitter.Tweet) {
		jsonRepresentation, err := json.Marshal(tweet)
		if capture(err) {
			return
		}
		fmt.Printf("New tweet from @%s : %s\n", tweet.User.ScreenName, jsonRepresentation)
		url := ""
		result := audd.RecognitionResult{}
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
			if !foundMedia && tweet.Entities != nil {
				if len(tweet.Entities.Urls) > 0 {
					for _, link := range tweet.Entities.Urls {
						if ContainsAnyString(link.ExpandedURL, []string{"instagram", "tiktok", "facebook"}) {
							url = link.ExpandedURL
						}
					}
				}
			}
			if tweet.InReplyToStatusID == 0 || foundMedia {
				break
			}
			tweet, _, err = client.Statuses.Show(tweet.InReplyToStatusID, &twitter.StatusShowParams{TweetMode: "extended"})
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
			fmt.Println("Recognizing from url ", url)
			result, err = auddClient.Recognize(url, "", nil)
			if capture(err) {
				return
			}
			tweet, _, err = client.Statuses.Show(tweet.ID, &twitter.StatusShowParams{TweetMode: "extended"})
			if capture(err) {
				return
			}
			if result.Title != "" {
				status := fmt.Sprintf("@%s Recognized! It's %s - %s\n\nListen: %s [plays on %s]",
					replyTo, result.Artist, result.Title, result.SongLink, result.Timecode)
				tweet, _, err = client.Statuses.Update(status, &twitter.StatusUpdateParams{InReplyToStatusID: replyToTweet})
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
	if err != nil {
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


func init() {
	/*err := raven.SetDSN("")
	if err != nil {
		panic(err)
	}*/
}

func capture(err error) bool {
	if err == nil {
		return false
	}
	_, file, no, ok := runtime.Caller(1)
	if ok {
		err = fmt.Errorf("%v from %s#%d", err, file, no)
	}
	//go raven.CaptureError(err, nil)
	return true
}
