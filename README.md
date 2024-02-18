# Apollo

> Apollo is a simple Music bot for Discord, designed to be easy to use and to be easy to host yourself.

## Getting Started

- You will need to [install docker](https://docs.docker.com/get-docker/) to run Apollo
- You will also need [install docker-compose](https://docs.docker.com/compose/install/) to run Apollo

Copy the contents of [docker-compose.yml](./docker-compose.yml) to a file called `docker-compose.yml` and then run `docker-compose up -d` to start the bot.

## Configuration

All of the configuration is done using environment variables.

### Required

- `DISCORD_TOKEN` - The token for your bot, you can get this from the [Discord Developer Portal](https://discord.com/developers/applications).

### Optional

- `GUILD_ID` - The ID of the guild you want the bot to sync commands to. (Likely only required if you are developing the bot)
