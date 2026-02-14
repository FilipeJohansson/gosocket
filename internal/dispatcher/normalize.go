// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

import (
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/message"
)

func normalizeMessage(
	msg *message.Message,
	opts normalizeOptions,
	serializers map[message.EncodingType]message.Serializer,
) (*message.Message, error) {
	if msg == nil {
		return nil, errors.ErrInvalidArguments
	}

	if msg.RawData == nil && msg.Data != nil {
		serializer := serializers[msg.Encoding]
		if serializer == nil {
			return nil, errors.ErrSerializerNotFound
		}

		raw, err := serializer.Marshal(msg.Data)
		if err != nil {
			return nil, err
		}

		msg.RawData = raw
	}

	// shallow copy to prevent mutation from user input
	m := *msg

	if m.Created.IsZero() {
		m.Created = time.Now()
	}

	if m.Encoding == 0 {
		m.Encoding = message.Raw
	}

	if opts.toClientID != "" {
		m.To = opts.toClientID
	}

	if opts.roomName != "" {
		m.Room = opts.roomName
	}

	return &m, nil
}

type normalizeOptions struct {
	toClientID string
	roomName   string
}
