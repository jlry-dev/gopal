package bot

import (
	"context"
	"sync"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

type queueImp struct {
	mu     sync.Mutex
	tracks []*lavalink.Track
}

type Queue interface {
	Push(track *lavalink.Track)
	Pop() *lavalink.Track
	PlayNext(ctx context.Context, player disgolink.Player)
}

func NewQueue() Queue {
	return &queueImp{
		tracks: []*lavalink.Track{},
	}
}

func (q *queueImp) Push(track *lavalink.Track) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.tracks = append(q.tracks, track)
}

func (q *queueImp) Pop() *lavalink.Track {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) < 1 {
		return nil
	}

	track := q.tracks[0]
	q.tracks = q.tracks[1:]

	return track
}

func (q *queueImp) PlayNext(ctx context.Context, player disgolink.Player) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) < 1 {
		return
	}

	track := q.tracks[0]
	q.tracks = q.tracks[1:]

	player.Update(ctx, lavalink.WithTrack(*track))
}

type queueManagerImp struct {
	mu     sync.RWMutex
	queues map[snowflake.ID]Queue
}

type QueueManager interface {
	Get(guildID snowflake.ID) Queue
	Remove(guildID snowflake.ID)
}

func NewQueueManager() QueueManager {
	return &queueManagerImp{
		queues: map[snowflake.ID]Queue{},
	}
}

func (qm *queueManagerImp) Get(guildID snowflake.ID) Queue {
	qm.mu.RLock()
	queue, ok := qm.queues[guildID]
	qm.mu.RUnlock()

	if ok {
		return queue
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()

	// another go routine might have created already
	// so need nato e check
	if queue, ok := qm.queues[guildID]; ok {
		return queue
	}

	queue = NewQueue()
	qm.queues[guildID] = queue
	return queue
}

func (qm *queueManagerImp) Remove(guildID snowflake.ID) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	delete(qm.queues, guildID)
}
