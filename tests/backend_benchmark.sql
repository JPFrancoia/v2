-- SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
-- SPDX-License-Identifier: Apache-2.0

\set ON_ERROR_STOP on

do $$
begin
    if current_database() <> 'miniflux_ui_benchmark' then
        raise exception 'The benchmark requires its dedicated database.';
    end if;
    if exists (select 1 from entries) then
        raise exception 'The benchmark requires an empty article table.';
    end if;
    if (select count(*) from users) <> 1 or not exists (select 1 from users where username = 'benchmark') then
        raise exception 'The benchmark requires its synthetic account.';
    end if;
end
$$;

update users set entries_per_page = 100, entry_order = 'score', entry_direction = 'desc',
    mark_read_on_view = true, show_score = true;

insert into feeds (id, user_id, category_id, title, feed_url, site_url, disabled)
select 100, u.id, c.id, 'Benchmark feed', 'https://example.invalid/feed', 'https://example.invalid', true
from users u join categories c on c.user_id = u.id order by c.id limit 1;

insert into entries (id, user_id, feed_id, hash, published_at, title, url, author, content,
    status, starred, saved_for_later, changed_at, score, raw_relevance_score, document_vectors)
select 1000 + n, u.id, 100, md5(n::text), now() - (n % 86400) * interval '1 second',
    'Benchmark article ' || n, 'https://example.invalid/article/' || n, 'Benchmark author',
    repeat('<p>Benchmark article body. Original content stays available.</p>', 48),
    case when n % 5 = 0 then 'read'::entry_status else 'unread'::entry_status end,
    n % 97 = 0 and n <> 0, n % 89 = 0 and n % 5 <> 0,
    now() - n * interval '1 second', n % 101, (n * 37 % 10000) / 100.0,
    to_tsvector('simple', 'benchmark article content')
from generate_series(0, :entries - 1) n cross join users u;

-- The web-only prediction reads use this synthetic Feedoscope-owned table.
create table entries_importance (
    entry_id bigint primary key references entries(id) on delete cascade,
    is_important boolean not null
);
insert into entries_importance (entry_id, is_important)
select id, id % 4 = 0 from entries;
analyze entries_importance;

insert into user_tags (id, user_id, title)
select n, u.id, 'Benchmark tag ' || n from generate_series(1, 10) n cross join users u;

insert into entry_user_tags (entry_id, user_tag_id)
select id, 1 + id % 10 from entries where id % 17 = 0 and id <> 1000;

select setval('entries_id_seq', (select max(id) from entries));
select setval('feeds_id_seq', 100);
select setval('user_tags_id_seq', 10);

analyze entries;
analyze feeds;
analyze user_tags;
analyze entry_user_tags;
