# Replay bot

This bot connects to your discord server (guild), joins the voice channel with the most members in it and listen to the audio streams.

**One** minute of audio stream is kept in memory and can be replayed by calling `/replay` .

## Configuration

### Creating the discord application

1. Create a [Discord application](https://discord.com/developers/applications)
2. Create a bot for this application.
3. Check "_Server Members Intent_". This bot uses it to find the channel with the most members connected to it. 
4. Invite the bot to your server:
   1. Go to the `OAuth > URL Generator` page.
   2. Check the following boxes:
      * `applications.commands`
      * `bot`
      * `Send Messages`
      * `Attach Files`
      * `Connect`
   3. Copy the URL to your browser and invite the bot to the correct server.


### Running the bot

It requires a few environment variables to be set:
#### Variable: `DISCORD_GUILD_ID`
> The discord server (guild) you want your bot to join and listen to.

##### To get the ID:
1. In discord app, right click on the picture of your server
2. Click on `Copy identifier`

Example: `123456789123456789`

#### Variable: `DISCORD_TOKEN`
> Secret token of your bot.

1. Go in the [developer portal of Discord](https://discord.com/developers/applications/).
2. Go to your application page.
3. Go to your bot page.
4. Press `Reset token`.
4. Copy the token.


Example: `ABCDEFGHIJKLMNOPQRSTUVWX.YzAbcD.EfGhIjKlMNoPQRsTuVwXyZaBcDeFGgjaldfa_a`
#### Running the bot


##### Option 1: Directly using go

You need **ffmpeg** to be installed and available in your _PATH_.
```sh
$ DISCORD_TOKEN=mytoken DISCORD_GUILD_ID=123 DEVELOPMENT=true run ./main.go
```

##### Option 2: Using docker
```sh
$ docker run -it --rm -e 'DISCORD_TOKEN=mytoken' -e 'DISCORD_GUILD_ID=123' -e 'DEVELOPMENT=true' ghcr.io/bonnetn/replay-discord-bot:main
```

_NOTE: `DEVELOPMENT=true` makes the logging a bit more friendly to human._
