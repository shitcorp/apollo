package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
	"github.com/rotisserie/eris"
)

type MusicBot struct {
	// Disgo client
	Client bot.Client

	// Lavalink client
	Lavalink disgolink.Client

	// Music queue manager
	Queues *QueueManager

	lavalinkNodes map[string]disgolink.Node
}

func NewMusicBot(token string) (*MusicBot, error) {
	// create wrapper for the bot
	musicBot := &MusicBot{

		// Create a new queue manager
		Queues: &QueueManager{
			// initialize the map of queues
			queues: make(map[snowflake.ID]*Queue),
		},

		lavalinkNodes: make(map[string]disgolink.Node),
	}

	client, err := disgo.New(token,
		bot.WithGatewayConfigOpts(
			// auto reconnect on disconnect
			gateway.WithAutoReconnect(true),

			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildVoiceStates,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagVoiceStates,
				cache.FlagRoles,
				cache.FlagMembers,
			),
		),
		// Register the command handler
		bot.WithEventListeners(musicBot.commandHandler()),
		// Register the ready event listener
		bot.WithEventListenerFunc(func(event *events.Ready) {
			logger.Info("Bot is ready")

			event.Client().SetPresence(context.TODO(), gateway.WithListeningActivity("music"))
		}),
		bot.WithEventListenerFunc(musicBot.onVoiceStateUpdate),
		bot.WithEventListenerFunc(musicBot.onVoiceServerUpdate),
		// Register the logger
		bot.WithLogger(logger),
	)
	if err != nil {
		return nil, err
	}
	musicBot.Client = client

	llclient := disgolink.New(client.ApplicationID(),
		disgolink.WithListenerFunc(musicBot.onPlayerPause),
		disgolink.WithListenerFunc(musicBot.onPlayerResume),
		disgolink.WithListenerFunc(musicBot.onTrackStart),
		disgolink.WithListenerFunc(musicBot.onTrackEnd),
		disgolink.WithListenerFunc(musicBot.onTrackException),
		disgolink.WithListenerFunc(musicBot.onTrackStuck),
		disgolink.WithListenerFunc(musicBot.onWebSocketClosed),
	)
	musicBot.Lavalink = llclient

	return musicBot, nil
}

func (b *MusicBot) onPlayerPause(player disgolink.Player, event lavalink.PlayerPauseEvent) {
	fmt.Printf("onPlayerPause: %v\n", event)
}

func (b *MusicBot) onPlayerResume(player disgolink.Player, event lavalink.PlayerResumeEvent) {
	fmt.Printf("onPlayerResume: %v\n", event)
}

func (b *MusicBot) onTrackStart(player disgolink.Player, event lavalink.TrackStartEvent) {
	fmt.Printf("onTrackStart: %v\n", event)
}

func (b *MusicBot) onTrackEnd(player disgolink.Player, event lavalink.TrackEndEvent) {
	if !event.Reason.MayStartNext() {
		return
	}

	queue := b.Queues.Get(event.GuildID())
	var (
		nextTrack lavalink.Track
		ok        bool
	)
	switch queue.Type {
	case QueueTypeNormal:
		nextTrack, ok = queue.Next()

	case QueueTypeRepeatTrack:
		nextTrack = *player.Track()

	case QueueTypeRepeatQueue:
		lastTrack, _ := b.Lavalink.BestNode().DecodeTrack(context.TODO(), event.Track.Encoded)
		queue.Add(*lastTrack)
		nextTrack, ok = queue.Next()
	}

	if !ok {
		return
	}
	if err := player.Update(context.TODO(), lavalink.WithTrack(nextTrack)); err != nil {
		logger.Error("Failed to play next track: ", err)
	}
}

func (b *MusicBot) onTrackException(player disgolink.Player, event lavalink.TrackExceptionEvent) {
	fmt.Printf("onTrackException: %v\n", event)
}

func (b *MusicBot) onTrackStuck(player disgolink.Player, event lavalink.TrackStuckEvent) {
	fmt.Printf("onTrackStuck: %v\n", event)
}

func (b *MusicBot) onWebSocketClosed(player disgolink.Player, event lavalink.WebSocketClosedEvent) {
	fmt.Printf("onWebSocketClosed: %v\n", event)
}

func (b *MusicBot) onVoiceStateUpdate(event *events.GuildVoiceStateUpdate) {
	if event.VoiceState.UserID != b.Client.ApplicationID() {
		return
	}
	b.Lavalink.OnVoiceStateUpdate(context.TODO(), event.VoiceState.GuildID, event.VoiceState.ChannelID, event.VoiceState.SessionID)
	if event.VoiceState.ChannelID == nil {
		b.Queues.Delete(event.VoiceState.GuildID)
	}
}

func (b *MusicBot) onVoiceServerUpdate(event *events.VoiceServerUpdate) {
	b.Lavalink.OnVoiceServerUpdate(context.TODO(), event.GuildID, event.Token, *event.Endpoint)
}

func (b *MusicBot) Start(ctx context.Context) error {
	logger.Info("Starting Apollo")

	connectCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	// open gateway connection
	err := b.Client.OpenGateway(connectCtx)
	if err != nil {
		msg := "error while opening gateway"
		logger.Error(msg, eris.Wrap(err, msg))
	}
	// defer b.Client.Close(ctx)

	node, err := b.Lavalink.AddNode(ctx, disgolink.NodeConfig{
		Name:     "test",
		Address:  "localhost:2333",
		Password: "youshallnotpass",
		Secure:   false,
	})
	if err != nil {
		msg := "error while adding lavalink node"
		err = eris.Wrap(err, msg)
		logger.Error(msg, err)
	}
	// defer node.Close()
	b.lavalinkNodes["test"] = node

	version, err := node.Version(ctx)
	if err != nil {
		msg := "error while getting lavalink node version"
		logger.Error(msg, eris.Wrap(err, msg))
	}
	logger.Info(fmt.Sprintf("Lavalink version: %s", version))

	return nil
}

func (b *MusicBot) Sync(ctx context.Context, guids []snowflake.ID) error {
	if len(guids) == 0 {
		logger.Info("Syncing commands for all guilds")
	} else {
		logger.Info("Syncing commands for specified guilds")
	}

	if err := handler.SyncCommands(b.Client, commands, guids, rest.WithCtx(ctx)); err != nil {
		return eris.Wrap(err, "error while syncing commands")
	}

	return nil
}

func (b *MusicBot) Close(ctx context.Context) {
	// close gateway connection
	b.Client.Close(ctx)

	for _, node := range b.lavalinkNodes {
		node.Close()
	}
}