package main

import (
	"encoding/json"
	"fmt"
	"github.com/AudDMusic/audd-go"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/getsentry/sentry-go"
	"log"
	"net/url"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const release = "twitter-bot@0.2.2"
const sentryDsn = ""

func ContainsAnyString(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
func GetSkipFirstFromLink(Url string) int {
	skip := 0
	if strings.HasSuffix(Url, ".m3u8") {
		return skip
	}
	u, err := url.Parse(Url)
	if err == nil {
		t := u.Query().Get("t")
		if t == "" {
			t = u.Query().Get("time_continue")
			if t == "" {
				t = u.Query().Get("start")
			}
		}
		if t != "" {
			t = strings.ToLower(strings.ReplaceAll(t, "s", ""))
			tInt := 0
			if strings.Contains(t, "m") {
				s := strings.Split(t, "m")
				tsInt, _ := strconv.Atoi(s[1])
				tInt += tsInt
				if strings.Contains(s[0], "h") {
					s := strings.Split(s[0], "h")
					if tmInt, err := strconv.Atoi(s[1]); !capture(err) {
						tInt += tmInt * 60
					}
					if thInt, err := strconv.Atoi(s[0]); !capture(err) {
						tInt += thInt * 60 * 60
					}
				} else {
					if tmInt, err := strconv.Atoi(s[0]); !capture(err) {
						tInt += tmInt * 60
					}
				}
			} else {
				if tsInt, err := strconv.Atoi(t); !capture(err) {
					tInt = tsInt
				}
			}
			skip += tInt
			fmt.Println("skip:", skip)
		}
	}
	return skip
}
func GetTimeFromText(s string) (int, int) {
	s = strings.ReplaceAll(s, " - ", "")
	words := strings.Split(s, " ")
	Time := 0
	TimeTo := 0
	maxScore := 0
	for _, w := range words {
		score := 0
		w2 := ""
		if strings.Contains(w, "-") {
			w2 = strings.Split(w, "-")[1]
			w = strings.Split(w, "-")[0]
			score += 1
		}
		w = strings.TrimSuffix(w, "s")
		w2 = strings.TrimSuffix(w2, "s")
		if strings.Contains(w, ":") {
			score += 2
		}
		if score > maxScore {
			t, err := TimeStringToSeconds(w)
			if err == nil {
				Time = t
				TimeTo, _ = TimeStringToSeconds(w2) // if w2 is empty or not a correct time, TimeTo is 0
				maxScore = score
			}
		}
	}
	return Time, TimeTo
}
func TimeStringToSeconds(s string) (int, error) {
	list := strings.Split(s, ":")
	if len(list) > 3 {
		return 0, fmt.Errorf("too many : thingies")
	}
	result, multiplier := 0, 1
	for i := len(list) - 1; i >= 0; i-- {
		c, err := strconv.Atoi(list[i])
		if err != nil {
			return 0, err
		}
		result += c * multiplier
		multiplier *= 60
	}
	return result, nil
}

const enterpriseChunkLength = 12

func SecondsToTimeString(i int, includeHours bool) string {
	if includeHours {
		return fmt.Sprintf("%02d:%02d:%02d", i/3600, (i%3600)/60, i%60)
	}
	return fmt.Sprintf("%02d:%02d", i/60, i%60)
}

func main() {
	var err error
	config := oauth1.NewConfig("", "")
	token := oauth1.NewToken("", "")
	auddClient := audd.NewClient("")
	// place your Twitter (consumer key, consumer secret, token, token secret) and AudD (api_token) credentials above
	
	auddClient.SetEndpoint(audd.EnterpriseAPIEndpoint)
	httpClient := config.Client(oauth1.NoContext, token)
	client := twitter.NewClient(httpClient)
	demux := twitter.NewSwitchDemux()
	demux.Tweet = func(tweet *twitter.Tweet) {
		jsonRepresentation, err := json.Marshal(tweet)
		if capture(err) {
			return
		}
		fmt.Printf("New tweet from @%s : %s\n", tweet.User.ScreenName, jsonRepresentation)
		resultUrl := ""
		foundMedia := false
		replyTo := tweet.User.ScreenName
		replyToTweet := tweet.ID
		for !foundMedia {
			if tweet.User != nil {
				if tweet.User.ScreenName == "MusicIDbot" {
					break
				}
			}
			if tweet.ExtendedEntities != nil {
				if len(tweet.ExtendedEntities.Media) > 0 {
					minBitrate := 0
					for _, video := range tweet.ExtendedEntities.Media[0].VideoInfo.Variants {
						if video.ContentType == "video/mp4" && (video.Bitrate < minBitrate || minBitrate == 0) {
							foundMedia = true
							resultUrl = video.URL
							minBitrate = video.Bitrate
						}
					}
				}
			}
			if !foundMedia && tweet.Entities != nil {
				if len(tweet.Entities.Urls) > 0 {
					resultUrl = tweet.Entities.Urls[0].ExpandedURL
					/*for _, link := range tweet.Entities.Urls {
						if ContainsAnyString(link.ExpandedURL, []string{"instagram", "tiktok", "facebook"}) {
							url = link.ExpandedURL
						}
					}*/
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
		if strings.HasPrefix(resultUrl, "https://lis.tn/") ||
			strings.HasPrefix(resultUrl, "https://audd.lis.tn/") {
			resultUrl = ""
		}
		if resultUrl != "" {
			limit := 3
			timestamp := GetSkipFirstFromLink(resultUrl)
			timestampTo := 0
			if timestamp == 0 {
				timestamp, timestampTo = GetTimeFromText(tweet.FullText)
			}
			if timestampTo != 0 && timestampTo-timestamp > limit*enterpriseChunkLength {
				// recognize music at the middle of the specified interval
				timestamp += (timestampTo - timestamp - limit*enterpriseChunkLength) / 2
			}
			timestampTo = timestamp + limit*enterpriseChunkLength
			atTheEnd := "false"
			if timestamp == 0 && strings.Contains(tweet.FullText, "at the end") {
				atTheEnd = "true"
			}
			fmt.Println("Recognizing from url ", resultUrl)
			results, err := auddClient.RecognizeLongAudio(resultUrl,
				map[string]string{"accurate_offsets": "true", "limit": "3", "reversed_order": atTheEnd,
					"skip_first_seconds": strconv.Itoa(timestamp)})
			response := ""
			if err != nil {
				if v, ok := err.(*audd.Error); ok {
					if v.ErrorCode == 501 {
						response = fmt.Sprintf("Sorry, I couldn't get any audio from the link (%s)", resultUrl)
					}
				}
				if response == "" {
					capture(err)
					return
				}
			}
			tweet, _, err = client.Statuses.Show(tweet.ID, &twitter.StatusShowParams{TweetMode: "extended"})
			if capture(err) {
				return
			}
			if len(results) != 0 {
				result := results[0].Songs[0]
				score := strconv.Itoa(result.Score) + "%"
				status := fmt.Sprintf("@%s It's %s by %s (%s; matched: %s)\n%s",
					replyTo, result.Title, result.Artist, result.Timecode, score, result.SongLink)
				tweet, _, err = client.Statuses.Update(status, &twitter.StatusUpdateParams{InReplyToStatusID: replyToTweet})
				if capture(err) {
					return
				}
			} else {
				if response == "" {
					at := SecondsToTimeString(timestamp, timestampTo >= 3600) + "-" + SecondsToTimeString(timestampTo, timestampTo >= 3600)
					if atTheEnd == "true" {
						at = "the end"
					}
					response = fmt.Sprintf("Sorry, I couldn't recognize the song."+
						"\n\nI tried to identify music from the %s at %s.",
						resultUrl, at)
				}
				status := fmt.Sprintf("@%s %s", replyTo, response)
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
		Track:         []string{"@MusicIDbot"},
		StallWarnings: twitter.Bool(true),
	}
	stream, err := client.Streams.Filter(filterParams)
	if err != nil {
		log.Println(err)
	}
	defer sentry.Recover()
	demux.HandleChan(stream.Messages)

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	fmt.Println("Stopping Stream...")
	stream.Stop()
}

func init() {
	err := sentry.Init(sentry.ClientOptions{
		// Either set your DSN here or set the SENTRY_DSN environment variable.
		Dsn:              sentryDsn,
		Release:          release,
		AttachStacktrace: true,
	})
	if err != nil {
		panic(err)
	}
}
func filterFrames(frames []sentry.Frame) []sentry.Frame {
	if len(frames) == 0 {
		return nil
	}
	filteredFrames := make([]sentry.Frame, 0, len(frames))
	for _, frame := range frames {
		if frame.Module == "runtime" || frame.Module == "testing" {
			continue
		}
		if frame.Module == "main" && strings.HasPrefix(frame.Function, "capture") {
			continue
		}
		filteredFrames = append(filteredFrames, frame)
	}
	return filteredFrames
}

func capture(err error) bool {
	if err == nil {
		return false
	}
	client, scope, event := captureGetEvent(err)
	go client.CaptureEvent(event, &sentry.EventHint{OriginalException: err}, scope)
	go fmt.Println(err.Error())
	return true
}

func captureGetEvent(err error) (*sentry.Client, *sentry.Scope, *sentry.Event) {
	extractFrames := func(pcs []uintptr) []sentry.Frame {
		var frames []sentry.Frame
		callersFrames := runtime.CallersFrames(pcs)
		for {
			callerFrame, more := callersFrames.Next()

			frames = append([]sentry.Frame{
				sentry.NewFrame(callerFrame),
			}, frames...)

			if !more {
				break
			}
		}
		return frames
	}
	GetStacktrace := func() *sentry.Stacktrace {
		pcs := make([]uintptr, 100)
		n := runtime.Callers(1, pcs)
		if n == 0 {
			return nil
		}
		frames := extractFrames(pcs[:n])
		frames = filterFrames(frames)
		stacktrace := sentry.Stacktrace{
			Frames: frames,
		}
		return &stacktrace
	}
	if err == nil {
		return nil, nil, nil
	}
	event := sentry.NewEvent()
	event.Exception = append(event.Exception, sentry.Exception{
		Value:      err.Error(),
		Type:       reflect.TypeOf(err).String(),
		Stacktrace: GetStacktrace(),
	})
	event.Level = sentry.LevelError
	hub := sentry.CurrentHub()
	return hub.Client(), hub.Scope(), event
}
