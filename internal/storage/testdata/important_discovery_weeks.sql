create temporary table entries_importance (
    entry_id bigint primary key,
    is_important boolean not null
);

-- The first six rows form the 3/6 cohort. The next rows check exclusions.
with cohort as (
    select date_trunc('week', current_timestamp at time zone 'America/Los_Angeles') - interval '1 week' as week_start
), samples (id, user_id, status, vote, starred, is_important, weeks_ago) as (
    values
        (201, 42, 'read', 1, false, true, 0),
        (202, 42, 'read', 1, false, true, 0),
        (203, 42, 'read', 1, false, true, 0),
        (204, 42, 'read', 0, false, true, 0),
        (205, 42, 'read', -1, true, true, 0),
        (206, 42, 'read', 0, true, true, 0),
        (207, 42, 'read', 1, false, false, 0),
        (208, 42, 'read', 0, true, null, 0),
        (209, 42, 'unread', 1, false, true, 0),
        (210, 42, 'removed', 1, false, true, 0),
        (211, 43, 'read', 1, false, true, 0),
        (212, 42, 'read', -1, false, true, 2),
        (213, 42, 'read', 1, false, true, 12)
), inserted as (
    insert into entries (id, user_id, feed_id, hash, published_at, title, url,
        changed_at, content, author, comments_url, status, vote, starred)
    select id, user_id, 9, md5(id::text), now(), 'Synthetic metric article',
        'https://example.test/metric/' || id,
        (week_start + interval '2 days' - weeks_ago * interval '1 week') at time zone 'America/Los_Angeles',
        '', '', '', status::entry_status, vote, starred
    from samples cross join cohort
    returning id
)
insert into entries_importance (entry_id, is_important)
select id, is_important from samples join inserted using (id) where is_important is not null;

-- Monday in UTC is still Sunday in Los Angeles.
insert into entries (id, user_id, feed_id, hash, published_at, title, url,
    changed_at, content, author, comments_url, status, vote, starred)
values (214, 42, 9, md5('214'), now(), 'Synthetic boundary article',
    'https://example.test/metric/214',
    (date_trunc('week', current_timestamp at time zone 'America/Los_Angeles') - interval '1 week' + interval '30 minutes') at time zone 'UTC',
    '', '', '', 'read', 1, false);
insert into entries_importance (entry_id, is_important) values (214, true);
