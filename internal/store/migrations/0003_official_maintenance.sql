-- Official announcements are independent of manual overrides.
CREATE TABLE maintenance_sources (
    server TEXT PRIMARY KEY CHECK (server = 'cn'),
    name TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    title TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    synced_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    CHECK ((starts_at IS NULL AND ends_at IS NULL) OR
           (starts_at IS NOT NULL AND ends_at IS NOT NULL AND ends_at > starts_at))
);
