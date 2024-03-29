package bot

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/lmittmann/tint"
	"github.com/rotisserie/eris"
)

// Global koanf instance. Use "_" as the key path delimiter. This can be "/" or any character.
var k = koanf.New(".")

// var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
var logger = slog.New(tint.NewHandler(os.Stdout, &tint.Options{
	Level:      slog.LevelInfo,
	TimeFormat: time.Kitchen,
}))

// starts the bot
func Start() {
	ctx := context.Background()

	// load nessessary config
	loadConfig()

	// create new music bot
	bot, err := NewMusicBot(k.String("discord.token"))
	if err != nil {
		msg := "error while creating disgo client"
		logger.Error(msg, eris.Wrap(err, msg))
	}

	// open gateway connection
	// and connect to lavalink
	err = bot.Start(ctx)
	if err != nil {
		logger.Error("error while starting bot", err)
	}

	guilds := []snowflake.ID{}

	if k.Exists("guild.id") {
		id := k.Int64("guild.id")
		logger.Info("guild_id is set, syncing only one guild", slog.Int64("guild.id", id))

		guilds = append(guilds, snowflake.ID(id))
	} else {
		logger.Info("guild.id is not set, syncing all guilds")
	}

	// sync commands to discord
	err = bot.Sync(ctx, guilds)
	if err != nil {
		logger.Error("error while syncing guilds", err)
	}

	logger.Info("DisGo example is now running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s

}

func loadConfig() {
	enableDotEnv := true

	// check if folder exists
	if _, err := os.Stat(".env"); eris.Is(err, os.ErrNotExist) {
		// if error is that file doesn't exist, then we don't want to enable dotenv
		enableDotEnv = false
	} else if err != nil {
		msg := "error while checking if .env file exists"
		logger.Error(msg, eris.Wrap(err, msg))
	}

	if enableDotEnv {
		// Load dotenv config.
		if err := k.Load(file.Provider(".env"), dotenv.Parser()); err != nil {
			msg := "error while loading dotenv config"
			logger.Error(msg, eris.Wrap(err, msg))
		}
	}

	// Load environment variables.
	if err := k.Load(env.Provider("", ".", func(str string) string {
		// convert env var names to lowercase and replace _ with .
		// konaf is case sensitive
		return strings.Replace(strings.ToLower(str), "_", ".", -1)
	}), nil); err != nil {
		msg := "error while loading environment variables"
		logger.Error(msg, eris.Wrap(err, msg))
	}

	// Set default values
	setDefaultConfig()
}

func setDefaultConfig() {
	// Set default config values

	// lavalink stuff
	if !k.Exists("lavalink.node.name") {
		k.Set("lavalink.node.name", "default")
	}
	if !k.Exists("lavalink.node.address") {
		logger.Warn("lavalink.node.address is not set, using default value", slog.String("lavalink.node.address", "localhost:2333"))
		k.Set("lavalink.node.address", "localhost:2333")
	}
	if !k.Exists("lavalink.node.password") {
		k.Set("lavalink.node.password", "youshallnotpass")
	}
}
