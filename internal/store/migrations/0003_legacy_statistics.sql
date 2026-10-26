-- Historical public AP JSON reports contain no screenshots. Preserve their
-- provenance separately; they do not impersonate authenticated OCR results.
CREATE TABLE legacy_azurstat_reports (
    id BIGSERIAL PRIMARY KEY,
    request_key TEXT UNIQUE NOT NULL,
    content_hash TEXT NOT NULL,
    device_id TEXT NOT NULL,
    task TEXT NOT NULL,
    zone TEXT NOT NULL DEFAULT '',
    zone_type TEXT NOT NULL DEFAULT '',
    zone_id INTEGER NOT NULL DEFAULT 0,
    hazard_level INTEGER NOT NULL CHECK(hazard_level BETWEEN 1 AND 6),
    combat_count BIGINT NOT NULL CHECK(combat_count>0),
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX legacy_azurstat_filter ON legacy_azurstat_reports(task,occurred_at,device_id,zone_id,hazard_level);
CREATE TABLE legacy_azurstat_items (
    report_id BIGINT NOT NULL REFERENCES legacy_azurstat_reports(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    item TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK(amount>0),
    is_meow BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY(report_id,ordinal)
);
CREATE INDEX stat_results_legacy_device_scene ON stat_results
    ((payload#>>'{source,metadata,device_id}'),(payload#>>'{source,metadata,scene}'),sha256)
    WHERE status='complete';

CREATE VIEW legacy_report_view AS
SELECT 'client:'||id::text AS report_key,device_id,task,zone,zone_type,zone_id,hazard_level,combat_count,occurred_at,'client'::text AS origin,'cn'::text AS server
FROM legacy_azurstat_reports
UNION ALL
SELECT DISTINCT ON(r.sha256) r.sha256 AS report_key,
    COALESCE(NULLIF(r.payload#>>'{source,metadata,legacy_device_id}',''),r.payload#>>'{source,metadata,device_id}') AS device_id,
    e.scene AS task,
    COALESCE(r.payload->'events'->e.ordinal->>'zone','') AS zone,
    COALESCE(r.payload->'events'->e.ordinal->>'zone_type','UNKNOWN') AS zone_type,
    COALESCE((r.payload->'events'->e.ordinal->>'zone_id')::integer,0) AS zone_id,
    COALESCE((r.payload->'events'->e.ordinal->>'hazard_level')::integer,0) AS hazard_level,
    COALESCE((r.payload#>>'{source,metadata,combat_count}')::bigint,0) AS combat_count,
    e.occurred_at,'image'::text AS origin,e.server
FROM stat_results r JOIN stat_events e ON e.sha256=r.sha256
WHERE r.status='complete' AND e.kind='drop' AND e.scene LIKE 'opsi\_%' ESCAPE '\';

CREATE VIEW legacy_item_view AS
SELECT 'client:'||report_id::text AS report_key,ordinal::bigint AS ordinal,item,amount,is_meow,CASE WHEN is_meow THEN 'meow' ELSE '' END AS tag
FROM legacy_azurstat_items
UNION ALL
SELECT e.sha256 AS report_key,(e.ordinal::bigint*1000000+row_number() OVER(PARTITION BY e.sha256,e.ordinal ORDER BY i.item,i.tag)) AS ordinal,
    i.item,i.amount,i.tag='meow' AS is_meow,i.tag
FROM stat_events e JOIN stat_items i ON i.sha256=e.sha256 AND i.ordinal=e.ordinal
WHERE e.kind='drop' AND e.scene LIKE 'opsi\_%' ESCAPE '\';
