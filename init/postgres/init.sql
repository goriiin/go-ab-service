CREATE TABLE IF NOT EXISTS experiments (
                                           id TEXT PRIMARY KEY,
                                           layer_id TEXT NOT NULL,
                                           config_version TEXT NOT NULL,
                                           end_time TIMESTAMPTZ,
                                           salt TEXT NOT NULL,
                                           status TEXT NOT NULL,
                                           targeting_rules JSONB,
                                           override_lists JSONB,
                                           variants JSONB
);

-- Индекс для быстрого поиска экспериментов по статусу (например, 'ACTIVE')
CREATE INDEX IF NOT EXISTS idx_experiments_status ON experiments (status);

CREATE TABLE IF NOT EXISTS outbox (
                                      event_id UUID PRIMARY KEY,
                                      aggregate_id TEXT NOT NULL,
                                      event_type TEXT NOT NULL,
                                      payload JSONB NOT NULL,
                                      created_at TIMESTAMPTZ NOT NULL,
                                      processing_state TEXT NOT NULL -- e.g., PENDING, LOCKED
);

-- Индекс для быстрого поиска событий, ожидающих обработки
CREATE INDEX IF NOT EXISTS idx_outbox_processing_state ON outbox (processing_state);
