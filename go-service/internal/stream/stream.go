package stream

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Mitesh0007/tracelight-go/internal/ingestion"
	"github.com/Mitesh0007/tracelight-go/internal/metrics"
)

type Redis struct {
	Client   *redis.Client
	Stream   string
	Group    string
	Consumer string
}

type OverflowMessage struct {
	ID      string
	Payload ingestion.TracePayload
}

var _ ingestion.RedisOverflowPublisher = (*Redis)(nil)

func (rs *Redis) CreateGroup(
	ctx context.Context,
) error {
	err := rs.Client.XGroupCreateMkStream(
		ctx,
		rs.Stream,
		rs.Group,
		"0",
	).Err()

	if err != nil &&
		!strings.Contains(
			err.Error(),
			"BUSYGROUP",
		) {
		return err
	}

	return nil
}

func (rs *Redis) AddTrace(
	ctx context.Context,
	payload ingestion.TracePayload,
) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return rs.Client.XAdd(
		ctx,
		&redis.XAddArgs{
			Stream: rs.Stream,
			Values: map[string]interface{}{
				"payload": string(data),
			},
		},
	).Result()
}

func (rs *Redis) ReadOverflow(
	ctx context.Context,
) ([]OverflowMessage, error) {
	streams, err := rs.Client.XReadGroup(
		ctx,
		&redis.XReadGroupArgs{
			Group:    rs.Group,
			Consumer: rs.Consumer,
			Streams: []string{
				rs.Stream,
				">",
			},
			Count: 10,
			Block: 5 * time.Second,
		},
	).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, err
	}

	messages := make([]OverflowMessage, 0)

	for _, stream := range streams {
		for _, message := range stream.Messages {
			payloadValue, ok := message.Values["payload"].(string)
			if !ok {
				metrics.TraceEventsRejectedTotal.
					WithLabelValues("redis_invalid_payload").
					Inc()

				log.Printf(
					"redis message %s has invalid payload",
					message.ID,
				)

				continue
			}

			var payload ingestion.TracePayload

			if err := json.Unmarshal(
				[]byte(payloadValue),
				&payload,
			); err != nil {
				metrics.TraceEventsRejectedTotal.
					WithLabelValues("redis_invalid_json").
					Inc()

				log.Printf(
					"redis message %s has invalid JSON: %v",
					message.ID,
					err,
				)

				continue
			}

			messages = append(
				messages,
				OverflowMessage{
					ID:      message.ID,
					Payload: payload,
				},
			)
		}
	}

	return messages, nil
}

func (rs *Redis) StartDrain(
	ctx context.Context,
	traceChan chan<- ingestion.TraceJob,
) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			default:
			}

			messages, err := rs.ReadOverflow(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}

				log.Printf(
					"redis overflow read failed: %v",
					err,
				)

				time.Sleep(time.Second)
				continue
			}

			for _, message := range messages {
				job := ingestion.TraceJob{
					Payload:        message.Payload,
					RedisMessageID: message.ID,
					Source:         "redis_overflow",
				}

				select {
				case traceChan <- job:
					metrics.TraceQueueDepth.Set(
						float64(len(traceChan)),
					)

				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

func (rs *Redis) Ack(
	ctx context.Context,
	messageID string,
) error {
	return rs.Client.XAck(
		ctx,
		rs.Stream,
		rs.Group,
		messageID,
	).Err()
}
