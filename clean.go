package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	flagToken = flag.String("token", "", "API key")
	flagUsers = flag.String("users", "", "users to delete")
)

type Channel struct {
	ID   string
	Name string

	// TODO this is a 'skip' flag for now
	Clean bool `json:",omitempty"`
	// Earliest seen entry ID while iterating backwards through the channel's history.
	LastProcessed string `json:",omitempty"`
}

// State is cross-invocation state persisted as JSON.
type State struct {
	Guild    string
	Channels []*Channel
}

type Session struct {
	discord *discordgo.Session
	state   *State
	// Delete entries before this timestamp.
	deleteBefore time.Time
}

func loadState() (*State, error) {
	state := &State{}
	f, err := os.Open("state.json")
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *State) save() error {
	f, err := os.Create("state.json.tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename("state.json.tmp", "state.json"); err != nil {
		return err
	}
	return nil
}

func (s *Session) getChannels() error {
	if s.state.Guild != "" && s.state.Channels != nil {
		return nil
	}

	guilds, err := s.discord.UserGuilds(100, "", "", false)
	if err != nil {
		return err
	}
	if len(guilds) != 1 {
		return fmt.Errorf("expected exactly one guild, got %d", len(guilds))
	}

	guild := guilds[0]
	log.Print("guild:", guild)
	s.state.Guild = guild.ID

	chans, err := s.discord.GuildChannels(guild.ID)
	if err != nil {
		return err
	}
	for _, dchan := range chans {
		log.Print("chan:", dchan)
		schan := &Channel{
			ID:   dchan.ID,
			Name: dchan.Name,
		}
		s.state.Channels = append(s.state.Channels, schan)
	}

	return s.state.save()
}

func topUsers(msgs []*discordgo.Message) string {
	hist := map[string]int{}
	for _, msg := range msgs {
		hist[msg.Author.String()] += 1
	}

	type kv struct {
		user  string
		count int
	}
	histArr := []kv{}
	for user, count := range hist {
		histArr = append(histArr, kv{user: user, count: count})
	}

	sort.Slice(histArr, func(i, j int) bool { return histArr[i].count > histArr[j].count })

	out := ""
	for i, kv := range histArr {
		out += fmt.Sprintf(" %s:%d", kv.user, kv.count)
		if i > 5 {
			break
		}
	}
	return out
}

func (s *Session) cleanChannel(ch *Channel) error {
	if ch.Clean {
		return nil
	}

	log.Printf("cleaning #%s", ch.Name)
	var lastStamp time.Time

	for {
		msgs, err := s.discord.ChannelMessages(ch.ID, 100, ch.LastProcessed, "", "")
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}

		top := topUsers(msgs)
		log.Println("top users:", top)

		for _, msg := range msgs {
			author := msg.Author.String()
			if msg.Timestamp.Before(s.deleteBefore) {
				log.Println("deleting", author, msg.ID, msg.Timestamp.String())
				if err := s.discord.ChannelMessageDelete(ch.ID, msg.ID); err != nil {
					return err
				}
			}
			ch.LastProcessed = msg.ID
			lastStamp = msg.Timestamp
		}

		// Hack: don't traverse before this arbitrary stopping point, just so we
		// don't run through the history of the world each time.
		if lastStamp.Before(time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)) {
			break
		}

		if err := s.state.save(); err != nil {
			return err
		}
	}

	return nil
}

func run() error {
	flag.Parse()

	state, err := loadState()
	if err != nil {
		return err
	}
	discord, err := discordgo.New("Bot " + *flagToken)
	if err != nil {
		return err
	}
	discord.LogLevel = discordgo.LogInformational
	//discord.Debug = true
	sess := &Session{
		discord:      discord,
		state:        state,
		deleteBefore: time.Now().AddDate(0, -1, 0),
	}

	if err := sess.getChannels(); err != nil {
		return err
	}

	for _, ch := range sess.state.Channels {
		if ch.Clean {
			continue
		}
		if err := sess.cleanChannel(ch); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal("ERROR:", err)
	}
}
