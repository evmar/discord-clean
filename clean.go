package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	flagToken = flag.String("token", "", "API key")
	flagUsers = flag.String("users", "", "users to delete (in the format 'foo#1234,bar#213')")
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
	// Delete entries from these users.
	deleteUsers map[string]bool
	// Time our last RPC was sent; used to self rate limit.
	lastRPC time.Time
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

func (s *Session) rpcWait() {
	delta := time.Since(s.lastRPC)
	if delta < 1*time.Second {
		delta = (1 * time.Second) - delta
		log.Println("sleeping", delta)
		time.Sleep(delta)
	}
	s.lastRPC = time.Now()
}

func (s *Session) getChannels() error {
	if s.state.Guild != "" && s.state.Channels != nil {
		return nil
	}

	guilds, err := s.discord.UserGuilds(100, "", "")
	if err != nil {
		return err
	}
	var qguild *discordgo.UserGuild
	for _, guild := range guilds {
		log.Print("guild:", guild)
		qguild = guild
	}
	s.state.Guild = qguild.ID

	chans, err := s.discord.GuildChannels(qguild.ID)
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

func (s *Session) cleanChannel(ch *Channel) error {
	if ch.Clean {
		return nil
	}

	log.Printf("cleaning #%s", ch.Name)
	var lastStamp string

	for {
		s.rpcWait()
		log.Println("getting messages before", ch.LastProcessed, lastStamp)
		msgs, err := s.discord.ChannelMessages(ch.ID, 100, ch.LastProcessed, "", "")
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, msg := range msgs {
			author := msg.Author.String()
			if msg.Timestamp.Before(s.deleteBefore) && s.deleteUsers[author] {
				s.rpcWait()
				log.Println("deleting", author, msg.ID, msg.Timestamp.String())
				if err := s.discord.ChannelMessageDelete(ch.ID, msg.ID); err != nil {
					return err
				}
			}
			ch.LastProcessed = msg.ID
			lastStamp = msg.Timestamp.String()
		}

		if err := s.state.save(); err != nil {
			return err
		}
	}

	return nil
}

func run() error {
	flag.Parse()

	users := map[string]bool{}
	for _, user := range strings.Split(*flagUsers, ",") {
		users[user] = true
	}

	state, err := loadState()
	if err != nil {
		return err
	}
	discord, err := discordgo.New("Bot " + *flagToken)
	if err != nil {
		return err
	}
	sess := &Session{
		discord:      discord,
		state:        state,
		deleteBefore: time.Now().AddDate(0, -1, 0),
		deleteUsers:  users,
		lastRPC:      time.Now().AddDate(-1, 0, 0),
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
