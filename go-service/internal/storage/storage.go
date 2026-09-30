package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Mitesh0007/tracelight-go/internal/ingestion"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Postgres struct {
	DB *sql.DB
}

func NewPostgres(
	dsn string,
) (*Postgres, error) {
	if dsn == "" {
		return nil, errors.New("postgres DSN is empty")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Postgres{
		DB: db,
	}, nil
}

func (ps *Postgres) Close() error {
	if ps == nil || ps.DB == nil {
		return nil
	}

	return ps.DB.Close()
}

func (ps *Postgres) CreateTables(
	ctx context.Context,
) error {
	_, err := ps.DB.ExecContext(
		ctx,
		`
		CREATE TABLE IF NOT EXISTS traces (
			trace_id TEXT PRIMARY KEY,
			provider_response_id TEXT,
			model TEXT NOT NULL,
			prompt_tokens INTEGER,
			completion_tokens INTEGER,
			total_tokens INTEGER,
			duration_ms INTEGER NOT NULL,
			success BOOLEAN NOT NULL,
			error JSONB,
			timestamp TIMESTAMPTZ NOT NULL
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("create traces table: %w", err)
	}

	return nil
}

func (ps *Postgres) SaveTrace(
	ctx context.Context,
	payload ingestion.TracePayload,
) error {
	return ps.SaveTraces(
		ctx,
		[]ingestion.TracePayload{payload},
	)
}

func (ps *Postgres) SaveTraces(
	ctx context.Context,
	payloads []ingestion.TracePayload,
) error {
	if len(payloads) == 0 {
		return nil
	}

	const columnsPerRow = 10

	valuePlaceholders := make(
		[]string,
		0,
		len(payloads),
	)
	args := make(
		[]any,
		0,
		len(payloads)*columnsPerRow,
	)

	for i, payload := range payloads {
		var errorData []byte

		if payload.Error != nil {
			data, err := json.Marshal(payload.Error)
			if err != nil {
				return fmt.Errorf("marshal trace error: %w", err)
			}

			errorData = data
		}

		base := i * columnsPerRow

		placeholders := make(
			[]string,
			columnsPerRow,
		)

		for column := 0; column < columnsPerRow; column++ {
			placeholders[column] = fmt.Sprintf(
				"$%d",
				base+column+1,
			)
		}

		valuePlaceholders = append(
			valuePlaceholders,
			"("+strings.Join(placeholders, ", ")+")",
		)

		args = append(
			args,
			payload.TraceID,
			payload.ProviderResponseID,
			payload.Model,
			payload.PromptTokens,
			payload.CompletionTokens,
			payload.TotalTokens,
			payload.DurationMs,
			payload.Success,
			errorData,
			payload.Timestamp,
		)
	}

	query := fmt.Sprintf(
		`
		INSERT INTO traces (
			trace_id,
			provider_response_id,
			model,
			prompt_tokens,
			completion_tokens,
			total_tokens,
			duration_ms,
			success,
			error,
			timestamp
		)
		VALUES %s
		ON CONFLICT (trace_id) DO UPDATE SET
			provider_response_id = EXCLUDED.provider_response_id,
			model = EXCLUDED.model,
			prompt_tokens = EXCLUDED.prompt_tokens,
			completion_tokens = EXCLUDED.completion_tokens,
			total_tokens = EXCLUDED.total_tokens,
			duration_ms = EXCLUDED.duration_ms,
			success = EXCLUDED.success,
			error = EXCLUDED.error,
			timestamp = EXCLUDED.timestamp
		`,
		strings.Join(valuePlaceholders, ",\n\t\t\t"),
	)

	_, err := ps.DB.ExecContext(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return fmt.Errorf("save traces batch: %w", err)
	}

	return nil
}

func (ps *Postgres) ListTraces(
	ctx context.Context,
) ([]ingestion.TracePayload, error) {
	rows, err := ps.DB.QueryContext(
		ctx,
		`
		SELECT
			trace_id,
			provider_response_id,
			model,
			prompt_tokens,
			completion_tokens,
			total_tokens,
			duration_ms,
			success,
			error,
			timestamp
		FROM traces
		ORDER BY timestamp DESC
		`,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	traces := make(
		[]ingestion.TracePayload,
		0,
	)

	for rows.Next() {
		var payload ingestion.TracePayload
		var errorData sql.NullString

		err := rows.Scan(
			&payload.TraceID,
			&payload.ProviderResponseID,
			&payload.Model,
			&payload.PromptTokens,
			&payload.CompletionTokens,
			&payload.TotalTokens,
			&payload.DurationMs,
			&payload.Success,
			&errorData,
			&payload.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		if errorData.Valid && errorData.String != "" {
			if err := json.Unmarshal(
				[]byte(errorData.String),
				&payload.Error,
			); err != nil {
				return nil, err
			}
		}

		traces = append(
			traces,
			payload,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return traces, nil
}

func (ps *Postgres) GetTrace(
	ctx context.Context,
	traceID string,
) (*ingestion.TracePayload, error) {
	var payload ingestion.TracePayload
	var errorData sql.NullString

	err := ps.DB.QueryRowContext(
		ctx,
		`
		SELECT
			trace_id,
			provider_response_id,
			model,
			prompt_tokens,
			completion_tokens,
			total_tokens,
			duration_ms,
			success,
			error,
			timestamp
		FROM traces
		WHERE trace_id = $1
		`,
		traceID,
	).Scan(
		&payload.TraceID,
		&payload.ProviderResponseID,
		&payload.Model,
		&payload.PromptTokens,
		&payload.CompletionTokens,
		&payload.TotalTokens,
		&payload.DurationMs,
		&payload.Success,
		&errorData,
		&payload.Timestamp,
	)
	if err != nil {
		return nil, err
	}

	if errorData.Valid && errorData.String != "" {
		if err := json.Unmarshal(
			[]byte(errorData.String),
			&payload.Error,
		); err != nil {
			return nil, err
		}
	}

	return &payload, nil
}
