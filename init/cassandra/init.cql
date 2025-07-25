CREATE TABLE IF NOT EXISTS ab_platform.experiments (
                                                       id text PRIMARY KEY,
                                                       layer_id text,
                                                       config_version text,
                                                       end_time timestamp,
                                                       salt text,
                                                       status text,
                                                       targeting_rules text,
                                                       override_lists text,
                                                       variants text
);

CREATE TABLE IF NOT EXISTS ab_platform.outbox (
                                                  event_id timeuuid PRIMARY KEY,
                                                  aggregate_id text,
                                                  event_type text,
                                                  payload text,
                                                  created_at timestamp,
                                                  processing_state text /* Новое поле: PENDING, LOCKED, DONE */
);

/* Индекс для быстрого поиска событий, ожидающих обработки */
CREATE INDEX IF NOT EXISTS on ab_platform.outbox (processing_state);

CREATE INDEX IF NOT EXISTS on ab_platform.experiments (status);