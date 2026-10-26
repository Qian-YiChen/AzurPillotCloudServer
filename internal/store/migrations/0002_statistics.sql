CREATE TABLE stat_results (
    sha256 TEXT PRIMARY KEY CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    revision BIGINT NOT NULL CHECK (revision > 0),
    status TEXT NOT NULL CHECK (status IN ('complete','needs_review','invalid')),
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE stat_result_versions (
    sha256 TEXT NOT NULL REFERENCES stat_results(sha256),
    revision BIGINT NOT NULL,
    content_hash TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (sha256,revision)
);
CREATE TABLE stat_events (
    sha256 TEXT NOT NULL REFERENCES stat_results(sha256),
    ordinal INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('research','drop')),
    event_type TEXT NOT NULL CHECK (event_type IN ('refresh','complete','drop')),
    occurred_at TIMESTAMPTZ NOT NULL,
    server TEXT NOT NULL,
    scene TEXT NOT NULL,
    series INTEGER NOT NULL DEFAULT 0,
    project TEXT NOT NULL DEFAULT '',
    focus_series INTEGER NOT NULL DEFAULT 0,
    duration_seconds BIGINT NOT NULL DEFAULT 0 CHECK (duration_seconds>=0),
    PRIMARY KEY (sha256,ordinal)
);
CREATE INDEX stat_events_filter ON stat_events(kind,occurred_at,server,scene,series,project);
CREATE TABLE stat_items (
    sha256 TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    item TEXT NOT NULL,
    tag TEXT NOT NULL DEFAULT '',
    amount BIGINT NOT NULL CHECK (amount>0),
    PRIMARY KEY (sha256,ordinal,item,tag),
    FOREIGN KEY (sha256,ordinal) REFERENCES stat_events(sha256,ordinal) ON DELETE CASCADE
);
CREATE TABLE template_candidates (
    id TEXT PRIMARY KEY,
    scene TEXT NOT NULL,
    image TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','assigned','ignored')),
    metadata JSONB NOT NULL DEFAULT '{}',
    first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE candidate_sources (
    candidate_id TEXT NOT NULL REFERENCES template_candidates(id),
    sample_sha256 TEXT NOT NULL,
    occurrence TEXT NOT NULL,
    metadata JSONB NOT NULL,
    PRIMARY KEY (candidate_id,sample_sha256,occurrence)
);
CREATE TABLE stat_templates (
    id TEXT PRIMARY KEY REFERENCES template_candidates(id),
    item_id TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK(category IN ('ship_blueprint','equipment_blueprint','consumable','other')),
    rarity INTEGER NOT NULL DEFAULT 0,
    series INTEGER NOT NULL DEFAULT 0,
    scene TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    image TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE template_releases (
    id BIGSERIAL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    payload BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE template_active (id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(id), release_id BIGINT REFERENCES template_releases(id));
INSERT INTO template_active(id) VALUES(TRUE);
CREATE TABLE worker_commands (
    id BIGSERIAL PRIMARY KEY,
    action TEXT NOT NULL CHECK(action IN ('reprocess','test')),
    payload JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','done','failed')),
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE stat_workers (id TEXT PRIMARY KEY, payload JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
