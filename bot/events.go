package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/asaskevich/EventBus"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/bcneng/candebot/cmd"
	"github.com/bcneng/candebot/inclusion"
	"github.com/bcneng/candebot/slackx"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/slack-go/slack/slackevents"
)

type MessageEvent interface {
	slackevents.AppMentionEvent | slackevents.AppHomeOpenedEvent | slackevents.AppUninstalledEvent |
		slackevents.ChannelCreatedEvent | slackevents.ChannelDeletedEvent | slackevents.ChannelArchiveEvent |
		slackevents.ChannelUnarchiveEvent | slackevents.ChannelLeftEvent | slackevents.ChannelRenameEvent |
		slackevents.ChannelIDChangedEvent | slackevents.GroupDeletedEvent | slackevents.GroupArchiveEvent |
		slackevents.GroupUnarchiveEvent | slackevents.GroupLeftEvent | slackevents.GroupRenameEvent |
		slackevents.GridMigrationFinishedEvent | slackevents.GridMigrationStartedEvent | slackevents.LinkSharedEvent |
		slackevents.MessageEvent | slackevents.MemberJoinedChannelEvent | slackevents.MemberLeftChannelEvent |
		slackevents.PinAddedEvent | slackevents.PinRemovedEvent | slackevents.ReactionAddedEvent |
		slackevents.ReactionRemovedEvent | slackevents.TeamJoinEvent | slackevents.TokensRevokedEvent |
		slackevents.EmojiChangedEvent
}

type Bus chan slackevents.EventsAPIInnerEvent

type EventHandler[T MessageEvent] func(Context, T) error

func does() EventHandler[slackevents.EmojiChangedEvent] {
	return func(Context, slackevents.EmojiChangedEvent) error {

		log.Println("HELLO!!!!!!!!!!!!!!!!!!!!!")
		return nil
	}
}
func Foo(Context, slackevents.EmojiChangedEvent) error {
	return nil
}

func foo[T MessageEvent](m map[string]EventHandler[T]) {
	m["whatever"] = EventHandler[T](does())
	b := NewBus(nil, m)

	b <- slackevents.EmojiChangedEvent{
		Type: "whatever",
	}

	time.Sleep(time.Second)

}

func NewBus[T MessageEvent](ctx context.Context, handlers map[string]EventHandler[T]) Bus {
	bus := EventBus.New()
	bus.Subscribe("main:calculator", calculator)
	bus.Publish("main:calculator", 20, 40)
	bus.Unsubscribe("main:calculator", calculator)

	var bus Bus
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-bus:
				if !ok {
					return
				}

				if h, ok := getHandler(handlers, event); ok { // TODO ?
					//if h, ok := handlers["whatever"]; ok { // TODO ?
					h(Context{}, event)
				}
			}
		}
	}()

	return bus
}

func getHandler[T MessageEvent](handlers map[string]EventHandler[T], event T) (EventHandler[T], bool) {
	for k, v := range slackevents.EventsAPIInnerEventMapping {
		instance
		switch t := event.(type) {

		}
	}
}

func eventsAPIHandler(botCtx Context, eventBus Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		if err := botCtx.VerifyRequest(r, buf.Bytes()); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			log.Printf("Fail to verify SigningSecret: %v", err)
		}

		eventsAPIEvent, err := slackevents.ParseEvent(buf.Bytes(), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text")
			_, _ = w.Write([]byte(r.Challenge))
		}

		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent

			switch event := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if event.BotID == botCtx.Config.Bot.ID {
					// Skip own (bot) command replies
					return
				}

				if event.SubType == "" || event.SubType == "message_replied" {
					// behaviors that apply to all messages posted by users both in channels or threads
					go checkLanguage(botCtx, event)
				}

				if event.ChannelType == "im" {
					log.Println("Direct message:", event.Text)
					botCommand(botCtx, SlackContext{
						User:            event.User,
						Channel:         event.Channel,
						Text:            event.Text,
						Timestamp:       event.TimeStamp,
						ThreadTimestamp: event.ThreadTimeStamp,
					})

					return
				}

				switch event.Channel {
				case botCtx.Config.Channels.Jobs:
					// Staff members are allowed to post messages
					if botCtx.IsStaff(event.User) {
						return
					}

					// Users are allowed to only post messages in threads
					if event.ThreadTimeStamp == "" {
						log.Printf("Someone wrote a random message in %s and will be removed. %s %s", event.Channel, event.Text, event.TimeStamp)
						_, _, _ = botCtx.AdminClient.DeleteMessage(event.Channel, event.TimeStamp)
						return
					}
				case botCtx.Config.Channels.Playground:
					// playground
				}
			case *slackevents.AppMentionEvent:
				log.Println("Mention message:", event.Text)
				botCommand(botCtx, SlackContext{
					User:            event.User,
					Channel:         event.Channel,
					Text:            event.Text,
					Timestamp:       event.TimeStamp,
					ThreadTimestamp: event.ThreadTimeStamp,
				})
			}
		}
	}
}

func botCommand(botCtx Context, slackCtx SlackContext) {
	text := strings.TrimSpace(strings.TrimPrefix(slackCtx.Text, fmt.Sprintf("<@%s>", botCtx.Config.Bot.UserID)))
	args := strings.Split(text, " ") // TODO strings.Split is not valid for quoted strings that contain spaces (E.g. echo command)
	w := bytes.NewBuffer([]byte{})
	defer func() {
		if w.Len() > 0 {
			_ = slackx.SendEphemeral(botCtx.Client, slackCtx.ThreadTimestamp, slackCtx.Channel, slackCtx.User, w.String())
		}
	}()

	_, kongCLI, err := cmd.NewCLI(botCtx.Config.Bot.Name, args, kong.Writers(w, w))
	if err != nil {
		return
	}

	log.Println("Running command:", text)
	if err := kongCLI.Run(botCtx, slackCtx); err != nil {
		_ = slackx.SendEphemeral(botCtx.Client, slackCtx.ThreadTimestamp, slackCtx.Channel, slackCtx.User, err.Error())
		return
	}
}

func checkLanguage(botCtx Context, event *slackevents.MessageEvent) {
	filter := inclusion.Filter(event.Text)
	if filter == nil {
		return
	}

	// Send reply as Slack ephemeral message
	_ = slackx.SendEphemeral(botCtx.Client, event.ThreadTimeStamp, event.Channel, event.User, filter.Reply)

	// Sending metrics
	botCtx.Harvester.RecordMetric(telemetry.Count{
		Name: "candebot.inclusion.message_filtered",
		Attributes: map[string]interface{}{
			"channel":          event.Channel,
			"filter":           filter.Filter,
			"candebot_version": botCtx.Version,
		},
		Value:     1,
		Timestamp: time.Now(),
	})
}
