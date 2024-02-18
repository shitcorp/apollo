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
	discord.SlashCommandCreate{
		Name:        "queue",
		Description: "Displays the current queue",
	},
	discord.SlashCommandCreate{
		Name:        "skip",
		Description: "Skips the current song",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "amount",
				Description: "The amount of songs to skip",
				Required:    false,
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
	r.Command("/now-playing", cmds.nowPlaying)
	r.Command("/pause", cmds.pause)
	r.Command("/stop", cmds.stop)
	r.Command("/disconnect", cmds.disconnect)
	r.Command("/volume", cmds.volume)
	r.Command("/shuffle", cmds.shuffle)
	r.Command("/queue", cmds.queue)
	r.Command("/skip", cmds.skip)

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

	var toPlay []lavalink.Track
	bestNode := h.musicBot.Lavalink.BestNode()

	// results, err := bestNode.LoadTracks(ctx, identifier)
	// if err != nil {
	// 	return err
	// }

	bestNode.LoadTracksHandler(ctx, identifier, disgolink.NewResultHandler(
		func(track lavalink.Track) {
			// _, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			// 	Content: json.Ptr(fmt.Sprintf("Loaded track: [`%s`](<%s>)", track.Info.Title, *track.Info.URI)),
			// })
			toPlay = append(toPlay, track)
		},
		func(playlist lavalink.Playlist) {
			// _, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			// 	Content: json.Ptr(fmt.Sprintf("Loaded playlist: `%s` with `%d` tracks", playlist.Info.Name, len(playlist.Tracks))),
			// })

			toPlay = append(toPlay, playlist.Tracks...)
		},
		func(tracks []lavalink.Track) {
			// _, _ = h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			// 	Content: json.Ptr(fmt.Sprintf("Loaded search result: [`%s`](<%s>)", tracks[0].Info.Title, *tracks[0].Info.URI)),
			// })

			// use first search result
			toPlay = append(toPlay, tracks[0])
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

	// check if there are any tracks to play
	if len(toPlay) == 0 {
		return nil
	}

	// join vc
	if err := h.musicBot.Client.UpdateVoiceState(context.TODO(), *event.GuildID(), voiceState.ChannelID, false, false); err != nil {
		return err
	}

	// get current player
	player := h.musicBot.Lavalink.Player(*event.GuildID())

	// get current track
	track := player.Track()

	var msg = ""

	// if there is no track playing, play first track
	if track == nil {

		track = &toPlay[0]
		// remove first track from queue
		toPlay = toPlay[1:]

		// play selected track
		err := player.Update(context.TODO(), lavalink.WithTrack(*track))
		if err != nil {
			logger.Error("Failed to play track", eris.Wrap(err, "failed to play track"))

			// notify user about error
			return event.CreateMessage(discord.MessageCreate{
				Content: fmt.Sprintf("Error while playing: `%s`", track.Info.Title),
			})
		}

		msg = fmt.Sprintf("Now playing: [`%s`](<%s>)", track.Info.Title, *track.Info.URI)
		logger.Info("Now playing track", slog.String("title", track.Info.Title), slog.String("uri", *track.Info.URI))

	}

	// add track(s) to queue
	queue := h.musicBot.Queues.Get(*event.GuildID())
	queue.Add(toPlay...)

	switch len(toPlay) {
	case 0:
		_, err := h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: &msg,
		})
		return err
	case 1:
		msg += fmt.Sprintf("\nAdded track to queue: [`%s`](<%s>)", toPlay[0].Info.Title, *toPlay[0].Info.URI)
		logger.Info("Added track to queue", slog.String("title", toPlay[0].Info.Title), slog.String("uri", *toPlay[0].Info.URI))
	default:
		msg += fmt.Sprintf("\nAdded `%d` tracks to queue", len(toPlay))
		logger.Info("Added tracks to queue", slog.Int("count", len(toPlay)))
	}

	_, err := h.musicBot.Client.Rest().UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
		Content: &msg,
	})
	return err
}

func (h CmdHandler) queue(event *handler.CommandEvent) error {
	queue := h.musicBot.Queues.Get(*event.GuildID())
	if queue == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	if len(queue.Tracks) == 0 {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No tracks in queue",
		})
	}

	var tracks string
	for i, track := range queue.Tracks {
		tracks += fmt.Sprintf("%d. [`%s`](<%s>)\n", i+1, track.Info.Title, *track.Info.URI)
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: fmt.Sprintf("Queue `%s`:\n%s", queue.Type, tracks),
	})
}

func (h CmdHandler) skip(event *handler.CommandEvent) error {
	logger.Info("Received /skip command")

	player := h.musicBot.Lavalink.ExistingPlayer(*event.GuildID())
	queue := h.musicBot.Queues.Get(*event.GuildID())
	if player == nil || queue == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	amount, ok := event.SlashCommandInteractionData().OptInt("amount")
	if !ok {
		amount = 1
	}
	logger.Info("Skipping tracks", slog.Int("amount", amount))

	track, ok := queue.Skip(amount)
	if !ok {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No tracks in queue",
		})
	}

	if err := player.Update(context.TODO(), lavalink.WithTrack(track)); err != nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error while skipping track: `%s`", err),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Skipped track",
	})
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

func (h CmdHandler) shuffle(event *handler.CommandEvent) error {
	queue := h.musicBot.Queues.Get(*event.GuildID())
	if queue == nil {
		return event.CreateMessage(discord.MessageCreate{
			Content: "No player found",
		})
	}

	queue.Shuffle()
	return event.CreateMessage(discord.MessageCreate{
		Content: "Queue shuffled",
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
