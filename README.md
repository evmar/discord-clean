This little program deletes some users' messages from a Discord channel.

I used it so Discord wouldn't have an archive of my old chats. Who can say
whether they actually delete them server-side, though!

## Server-side setup

Discord unfortunately does not let software act on behalf of a user directly,
but instead allows software to show up as "bots". You then must be a server
admin to be able to grant the bot user access to delete your messages.

1. Create a Discord "app" via the
   [developer portal](https://discord.com/developers/applications).
2. Bot -> Add Bot.
   1. Reset Token, then save the generated token for use below.
3. OAuth2 -> URL Generator.
   1. Tick `bot`.
   2. Add ... some set of OAuth2 scopes (I forget already, eek).
   3. Open the generated URL in your browser to add the bot to your server.

## Client-side execution

```
$ go build .
$ ./discord-clean -token="$TOKEN" -users="evmar#1234,jon#4231"
```
