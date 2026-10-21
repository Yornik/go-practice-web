-- The course grew from 25 exercises in 5 tiers to 50 in 10. The first
-- course's exercises are tiers 6 to 10 now, under new slugs: 01-logfmt is
-- 26-logfmt, ... 25-loadbalancer is 50-loadbalancer (the table is
-- internal/course/renames.go; a test checks that this file matches it).
-- Deploy this only with the bumped course: with the first course loaded,
-- the renamed rows point at exercises it does not have.
--
-- The old and the new slugs are disjoint sets, so no primary key collides.

UPDATE code SET exercise = CASE exercise
        WHEN '01-logfmt' THEN '26-logfmt'
        WHEN '02-accesslog' THEN '27-accesslog'
        WHEN '03-latency' THEN '28-latency'
        WHEN '04-grep' THEN '29-grep'
        WHEN '05-slo' THEN '30-slo'
        WHEN '06-envconfig' THEN '31-envconfig'
        WHEN '07-retry' THEN '32-retry'
        WHEN '08-ratelimit' THEN '33-ratelimit'
        WHEN '09-cache' THEN '34-cache'
        WHEN '10-deployorder' THEN '35-deployorder'
        WHEN '11-tracetree' THEN '36-tracetree'
        WHEN '12-workerpool' THEN '37-workerpool'
        WHEN '13-logmerge' THEN '38-logmerge'
        WHEN '14-circuitbreaker' THEN '39-circuitbreaker'
        WHEN '15-healthcheck' THEN '40-healthcheck'
        WHEN '16-middleware' THEN '41-middleware'
        WHEN '17-promtext' THEN '42-promtext'
        WHEN '18-certcheck' THEN '43-certcheck'
        WHEN '19-alertgroup' THEN '44-alertgroup'
        WHEN '20-singleflight' THEN '45-singleflight'
        WHEN '21-broadcaster' THEN '46-broadcaster'
        WHEN '22-reconciler' THEN '47-reconciler'
        WHEN '23-leaderelection' THEN '48-leaderelection'
        WHEN '24-diskbuffer' THEN '49-diskbuffer'
        WHEN '25-loadbalancer' THEN '50-loadbalancer'
    END
WHERE exercise IN (
    '01-logfmt', '02-accesslog', '03-latency', '04-grep', '05-slo',
    '06-envconfig', '07-retry', '08-ratelimit', '09-cache', '10-deployorder',
    '11-tracetree', '12-workerpool', '13-logmerge', '14-circuitbreaker', '15-healthcheck',
    '16-middleware', '17-promtext', '18-certcheck', '19-alertgroup', '20-singleflight',
    '21-broadcaster', '22-reconciler', '23-leaderelection', '24-diskbuffer', '25-loadbalancer'
);

UPDATE progress SET exercise = CASE exercise
        WHEN '01-logfmt' THEN '26-logfmt'
        WHEN '02-accesslog' THEN '27-accesslog'
        WHEN '03-latency' THEN '28-latency'
        WHEN '04-grep' THEN '29-grep'
        WHEN '05-slo' THEN '30-slo'
        WHEN '06-envconfig' THEN '31-envconfig'
        WHEN '07-retry' THEN '32-retry'
        WHEN '08-ratelimit' THEN '33-ratelimit'
        WHEN '09-cache' THEN '34-cache'
        WHEN '10-deployorder' THEN '35-deployorder'
        WHEN '11-tracetree' THEN '36-tracetree'
        WHEN '12-workerpool' THEN '37-workerpool'
        WHEN '13-logmerge' THEN '38-logmerge'
        WHEN '14-circuitbreaker' THEN '39-circuitbreaker'
        WHEN '15-healthcheck' THEN '40-healthcheck'
        WHEN '16-middleware' THEN '41-middleware'
        WHEN '17-promtext' THEN '42-promtext'
        WHEN '18-certcheck' THEN '43-certcheck'
        WHEN '19-alertgroup' THEN '44-alertgroup'
        WHEN '20-singleflight' THEN '45-singleflight'
        WHEN '21-broadcaster' THEN '46-broadcaster'
        WHEN '22-reconciler' THEN '47-reconciler'
        WHEN '23-leaderelection' THEN '48-leaderelection'
        WHEN '24-diskbuffer' THEN '49-diskbuffer'
        WHEN '25-loadbalancer' THEN '50-loadbalancer'
    END
WHERE exercise IN (
    '01-logfmt', '02-accesslog', '03-latency', '04-grep', '05-slo',
    '06-envconfig', '07-retry', '08-ratelimit', '09-cache', '10-deployorder',
    '11-tracetree', '12-workerpool', '13-logmerge', '14-circuitbreaker', '15-healthcheck',
    '16-middleware', '17-promtext', '18-certcheck', '19-alertgroup', '20-singleflight',
    '21-broadcaster', '22-reconciler', '23-leaderelection', '24-diskbuffer', '25-loadbalancer'
);

UPDATE reveals SET exercise = CASE exercise
        WHEN '01-logfmt' THEN '26-logfmt'
        WHEN '02-accesslog' THEN '27-accesslog'
        WHEN '03-latency' THEN '28-latency'
        WHEN '04-grep' THEN '29-grep'
        WHEN '05-slo' THEN '30-slo'
        WHEN '06-envconfig' THEN '31-envconfig'
        WHEN '07-retry' THEN '32-retry'
        WHEN '08-ratelimit' THEN '33-ratelimit'
        WHEN '09-cache' THEN '34-cache'
        WHEN '10-deployorder' THEN '35-deployorder'
        WHEN '11-tracetree' THEN '36-tracetree'
        WHEN '12-workerpool' THEN '37-workerpool'
        WHEN '13-logmerge' THEN '38-logmerge'
        WHEN '14-circuitbreaker' THEN '39-circuitbreaker'
        WHEN '15-healthcheck' THEN '40-healthcheck'
        WHEN '16-middleware' THEN '41-middleware'
        WHEN '17-promtext' THEN '42-promtext'
        WHEN '18-certcheck' THEN '43-certcheck'
        WHEN '19-alertgroup' THEN '44-alertgroup'
        WHEN '20-singleflight' THEN '45-singleflight'
        WHEN '21-broadcaster' THEN '46-broadcaster'
        WHEN '22-reconciler' THEN '47-reconciler'
        WHEN '23-leaderelection' THEN '48-leaderelection'
        WHEN '24-diskbuffer' THEN '49-diskbuffer'
        WHEN '25-loadbalancer' THEN '50-loadbalancer'
    END
WHERE exercise IN (
    '01-logfmt', '02-accesslog', '03-latency', '04-grep', '05-slo',
    '06-envconfig', '07-retry', '08-ratelimit', '09-cache', '10-deployorder',
    '11-tracetree', '12-workerpool', '13-logmerge', '14-circuitbreaker', '15-healthcheck',
    '16-middleware', '17-promtext', '18-certcheck', '19-alertgroup', '20-singleflight',
    '21-broadcaster', '22-reconciler', '23-leaderelection', '24-diskbuffer', '25-loadbalancer'
);

-- SQLite cannot change a CHECK constraint, so the table is rebuilt with
-- room for tier1 to tier10. A tier certificate of the first course moves
-- from tierN to tier(N+5), the tier that holds its exercises now. The
-- payload and the signature stay byte for byte: they are what was signed,
-- and the site reads the old kind and slugs in the payload through the
-- rename table.
--
-- The rename comes first: with foreign keys on, SQLite rewrites the
-- self-reference of replaced_by to the renamed table, so dropping it later
-- checks nothing against the new table.
DROP INDEX certificates_valid;
DROP INDEX certificates_user;
ALTER TABLE certificates RENAME TO certificates_v1;

CREATE TABLE certificates (
    id           TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL CHECK (kind IN ('tier1', 'tier2', 'tier3', 'tier4', 'tier5',
                                                  'tier6', 'tier7', 'tier8', 'tier9', 'tier10', 'course')),
    name_on_cert TEXT    NOT NULL,
    issued_at    TEXT    NOT NULL,
    show_stats   INTEGER NOT NULL DEFAULT 0,
    stats        TEXT,
    payload      TEXT    NOT NULL,
    signature    BLOB    NOT NULL,
    revoked_at   TEXT,
    replaced_by  TEXT    REFERENCES certificates (id)
);

-- replaced_by points at a row that may come later in the copy, so it is
-- set in a second pass, when every row exists.
INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, show_stats, stats, payload, signature, revoked_at)
SELECT id, user_id,
       CASE kind
           WHEN 'tier1' THEN 'tier6'
           WHEN 'tier2' THEN 'tier7'
           WHEN 'tier3' THEN 'tier8'
           WHEN 'tier4' THEN 'tier9'
           WHEN 'tier5' THEN 'tier10'
           ELSE kind
       END,
       name_on_cert, issued_at, show_stats, stats, payload, signature, revoked_at
FROM certificates_v1;

UPDATE certificates
SET replaced_by = (SELECT old.replaced_by FROM certificates_v1 AS old WHERE old.id = certificates.id)
WHERE id IN (SELECT id FROM certificates_v1 WHERE replaced_by IS NOT NULL);

DROP TABLE certificates_v1;

CREATE INDEX certificates_user ON certificates (user_id);
-- At most 1 valid certificate per user and kind; a re-issue revokes the old
-- one in the same transaction.
CREATE UNIQUE INDEX certificates_valid ON certificates (user_id, kind) WHERE revoked_at IS NULL;
