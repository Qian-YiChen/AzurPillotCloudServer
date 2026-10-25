-- 初始 schema：CL1 快照与去重、维护计划、公告、更新策略、发布历史。

CREATE TABLE cl1_snapshots (
    device_id          TEXT        NOT NULL,
    instance_id        TEXT        NOT NULL,
    month              TEXT        NOT NULL,
    collected_at       TIMESTAMPTZ NOT NULL,
    battle_count       BIGINT      NOT NULL,
    battle_rounds      BIGINT      NOT NULL,
    sortie_cost        BIGINT      NOT NULL,
    akashi_encounters  BIGINT      NOT NULL,
    akashi_ap          BIGINT      NOT NULL,
    akashi_probability DOUBLE PRECISION NOT NULL,
    average_stamina    DOUBLE PRECISION NOT NULL,
    net_stamina_gain   BIGINT      NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, instance_id, month)
);

-- 请求去重结果保留 24 小时（由服务端清理协程删除过期行）。
CREATE TABLE cl1_dedup (
    request_id  TEXT PRIMARY KEY,
    device_id   TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    body_hash   TEXT NOT NULL,
    response    JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX cl1_dedup_expires_idx ON cl1_dedup (expires_at);

CREATE TABLE maintenance_schedules (
    server     TEXT PRIMARY KEY CHECK (server IN ('cn', 'en', 'jp', 'tw')),
    name       TEXT NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE announcements (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    content    TEXT,
    url        TEXT,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX announcements_active_idx ON announcements (active, updated_at DESC);

-- 单行表：强制更新开关。
CREATE TABLE update_policy (
    id           BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    force_update BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO update_policy (id, force_update) VALUES (TRUE, FALSE)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE publish_history (
    id           BIGSERIAL PRIMARY KEY,
    worker       TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    status       TEXT NOT NULL,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX publish_history_created_idx ON publish_history (created_at DESC);
