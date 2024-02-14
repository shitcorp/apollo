package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/json"
)

var commands = []discord.ApplicationCommandCreate{
	discord.SlashCommandCreate{
		Name:        "play",
		Description: "Plays a song",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "identifier",
				Description: "The song link or search query",
				Required:    true,
			},
			discord.ApplicationCommandOptionString{
				Name:        "source",
				Description: "The source to search on",
				Required:    false,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{
						Name:  "YouTube",
						Value: string(lavalink.SearchTypeYouTube),
					},
					{
						Name:  "YouTube Music",
						Value: string(lavalink.SearchTypeYouTubeMusic),
					},
					{
						Name:  "SoundCloud",
						Value: string(lavalink.SearchTypeSoundCloud),
					},
					{
						Name:  "Deezer",
						Value: "dzsearch",
					},
					{
						Name:  "Deezer ISRC",
						Value: "dzisrc",
					},
					{
						Name:  "Spotify",
						Value: "spsearch",
					},
					{
						Name:  "AppleMusic",
						Value: "amsearch",
					},
				},
			},
		},
	},
}

var (
	urlPattern    = regexp.MustCompile("^https?://[-a-zA-Z0-9+&@#/%?=~_|!:,.;]*[-a-zA-Z0-9+&@#/%=~_|]?")
	searchPattern = regexp.MustCompile(`^(.{2})search:(.+)`)
)

func (b *MusicBot) commandHandler() *handler.Mux {
	r := handler.New()

	r.Command("/play", func(event *handler.CommandEvent) error {
		logger.Info("Received /play command")

		data := event.SlashCommandInteractionData()

		identifier := data.String("identifier")
		if source, ok := data.OptString("source"); ok {
			identifier = lavalink.SearchType(source).Apply(identifier)
		} else if !urlPattern.MatchString(identifier) && !searchPattern.MatchString(identifier) {
			identifier = lavalink.SearchTypeYouTube.Apply(identifier)
		}

		voiceState, ok := b.Client.Caches().VoiceState(*event.GuildID(), event.User().ID)
		if !ok {
			return event.CreateMessage(discord.MessageCreate{
				Content: "You need to be in a voice channel to use this command",
			})
		}

		if err := event.DeferCreateMessage(false); err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var toPlay *lavalink.Track
		bestNode := b.Lavalink.BestNode()

		// results, err := bestNode.LoadTracks(ctx, identifier)
		// if err != nil {
		// 	return err
		// }

		bestNode.LoadTracksHandler(ctx, identifier, disgolink.NewResultHandler(
			func(track lavalink.Track) {
				_, _ = b.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
					Content: json.Ptr(fmt.Sprintf("Loaded track: [`%s`](<%s>)", track.Info.Title, *track.Info.URI)),
				})
				toPlay = &track
			},
			func(playlist lavalink.Playlist) {
				_, _ = b.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
					Content: json.Ptr(fmt.Sprintf("Loaded playlist: `%s` with `%d` tracks", playlist.Info.Name, len(playlist.Tracks))),
				})
				toPlay = &playlist.Tracks[0]
			},
			func(tracks []lavalink.Track) {
				_, _ = b.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
					Content: json.Ptr(fmt.Sprintf("Loaded search result: [`%s`](<%s>)", tracks[0].Info.Title, *tracks[0].Info.URI)),
				})
				toPlay = &tracks[0]
			},
			func() {
				_, _ = b.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
					Content: json.Ptr(fmt.Sprintf("Nothing found for: `%s`", identifier)),
				})
			},
			func(err error) {
				_, _ = b.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
					Content: json.Ptr(fmt.Sprintf("Error while looking up query: `%s`", err)),
				})
			},
		))
		logger.Info("toPlay", slog.String("title", toPlay.Info.Title), slog.String("uri", *toPlay.Info.URI))

		if toPlay == nil {
			return nil
		}

		// join vc
		if err := b.Client.UpdateVoiceState(context.TODO(), *event.GuildID(), voiceState.ChannelID, false, false); err != nil {
			return err
		}

		player := b.Lavalink.Player(*event.GuildID())

		return player.Update(context.TODO(), lavalink.WithTrack(*toPlay))
	})

	return r
}
