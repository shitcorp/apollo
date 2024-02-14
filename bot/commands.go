package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/rotisserie/eris"
)

var slashCommands = []discord.ApplicationCommandCreate{
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

type CmdHandler struct {
	musicBot *MusicBot
}

// syncs slash commands to discord
func (b *MusicBot) Sync(ctx context.Context, guids []snowflake.ID) error {
	if len(guids) == 0 {
		logger.Info("Syncing commands for all guilds")
	} else {
		logger.Info("Syncing commands for specified guilds")
	}

	if err := handler.SyncCommands(b.Client, slashCommands, guids, rest.WithCtx(ctx)); err != nil {
		return eris.Wrap(err, "error while syncing commands")
	}

	return nil
}

func (b *MusicBot) buildCommandHandler() *handler.Mux {
	// create new command handler
	cmds := CmdHandler{musicBot: b}
	// create new handler mux
	r := handler.New()

	r.Command("/play", cmds.play)

	return r
}

// plays a song, or adds it to the queue if a song is already playing
func (h CmdHandler) play(event *handler.CommandEvent) error {
	logger.Info("Received /play command")

	data := event.SlashCommandInteractionData()

	identifier := data.String("identifier")
	if source, ok := data.OptString("source"); ok {
		identifier = lavalink.SearchType(source).Apply(identifier)
	} else if !urlPattern.MatchString(identifier) && !searchPattern.MatchString(identifier) {
		identifier = lavalink.SearchTypeYouTube.Apply(identifier)
	}

	voiceState, ok := h.musicBot.Client.Caches().VoiceState(*event.GuildID(), event.User().ID)
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
	bestNode := h.musicBot.Lavalink.BestNode()

	// results, err := bestNode.LoadTracks(ctx, identifier)
	// if err != nil {
	// 	return err
	// }

	bestNode.LoadTracksHandler(ctx, identifier, disgolink.NewResultHandler(
		func(track lavalink.Track) {
			_, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: json.Ptr(fmt.Sprintf("Loaded track: [`%s`](<%s>)", track.Info.Title, *track.Info.URI)),
			})
			toPlay = &track
		},
		func(playlist lavalink.Playlist) {
			_, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: json.Ptr(fmt.Sprintf("Loaded playlist: `%s` with `%d` tracks", playlist.Info.Name, len(playlist.Tracks))),
			})
			toPlay = &playlist.Tracks[0]
		},
		func(tracks []lavalink.Track) {
			_, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: json.Ptr(fmt.Sprintf("Loaded search result: [`%s`](<%s>)", tracks[0].Info.Title, *tracks[0].Info.URI)),
			})
			toPlay = &tracks[0]
		},
		func() {
			_, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: json.Ptr(fmt.Sprintf("Nothing found for: `%s`", identifier)),
			})
		},
		func(err error) {
			_, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: json.Ptr(fmt.Sprintf("Error while looking up query: `%s`", err)),
			})
		},
	))
	logger.Info("toPlay", slog.String("title", toPlay.Info.Title), slog.String("uri", *toPlay.Info.URI))

	if toPlay == nil {
		return nil
	}

	// join vc
	if err := h.musicBot.Client.UpdateVoiceState(context.TODO(), *event.GuildID(), voiceState.ChannelID, false, false); err != nil {
		return err
	}

	player := h.musicBot.Lavalink.Player(*event.GuildID())

	return player.Update(context.TODO(), lavalink.WithTrack(*toPlay))
}
