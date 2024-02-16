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
	discord.SlashCommandCreate{
		Name:        "now-playing",
		Description: "Shows the current playing song",
	},
	discord.SlashCommandCreate{
		Name:        "pause",
		Description: "Pauses/resumes the current song",
	},
	discord.SlashCommandCreate{
		Name:        "stop",
		Description: "Stops the current song and stops the player",
	},
	discord.SlashCommandCreate{
		Name:        "disconnect",
		Description: "Disconnects the player",
	},
	discord.SlashCommandCreate{
		Name:        "volume",
		Description: "Sets the volume of the player",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "volume",
				Description: "The volume to set",
				Required:    true,
				MaxValue:    json.Ptr(1000),
				MinValue:    json.Ptr(0),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "shuffle",
		Description: "Shuffles the current queue",
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
	r.Command("/now-playing", cmds.nowPlaying)
	r.Command("/pause", cmds.pause)
	r.Command("/stop", cmds.stop)
	r.Command("/disconnect", cmds.disconnect)
	r.Command("/volume", cmds.volume)

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

func (h CmdHandler) pause(event *handler.CommandEvent) error {
	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	if err := player.Update(context.TODO(), lavalink.WithPaused(!player.Paused())); err != nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error while pausing: `%s`", err),
		})
	}

	status := "playing"
	if player.Paused() {
		status = "paused"
	}
	return event.CreateMessage(discord.MessageCreate{
		Content: fmt.Sprintf("Player is now %s", status),
	})
}

func (h CmdHandler) volume(event *handler.CommandEvent) error {
	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	volume := event.SlashCommandInteractionData().Int("volume")
	if err := player.Update(context.TODO(), lavalink.WithVolume(volume)); err != nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error while setting volume: `%s`", err),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: fmt.Sprintf("Volume set to `%d`", volume),
	})
}

func (h CmdHandler) stop(event *handler.CommandEvent) error {
	logger.Info("Received /stop command")

	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	if err := player.Update(context.TODO(), lavalink.WithNullTrack()); err != nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error while stopping: `%s`", err),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Player stopped",
	})
}

func (h CmdHandler) disconnect(event *handler.CommandEvent) error {
	logger.Info("Received /disconnect command")

	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	if err := h.musicBot.Client.UpdateVoiceState(context.TODO(), *event.GuildID(), nil, false, false); err != nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error while disconnecting: `%s`", err),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Player disconnected",
	})
}

func (h CmdHandler) nowPlaying(event *handler.CommandEvent) error {
	logger.Info("Received /now-playing command")

	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	track := player.Track()
	if track == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No track found",
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: fmt.Sprintf("Now playing: [`%s`](<%s>)\n\n %s / %s", track.Info.Title, *track.Info.URI, formatPosition(player.Position()), formatPosition(track.Info.Length)),
	})
}

func formatPosition(position lavalink.Duration) string {
	if position == 0 {
		return "0:00"
	}
	return fmt.Sprintf("%d:%02d", position.Minutes(), position.SecondsPart())
}
