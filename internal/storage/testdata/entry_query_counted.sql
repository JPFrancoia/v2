create temporary table entry_user_tags (like public.entry_user_tags including defaults);
create temporary table enclosures (like public.enclosures including defaults);

insert into users (id, username, timezone) values (43, 'other_query_fixture', 'UTC');
insert into categories (id, user_id, title, hide_globally)
values (8, 42, 'Hidden fixture category', true), (9, 43, 'Other fixture category', false);
insert into feeds (id, user_id, category_id, title, feed_url, site_url,
    description, scraper_rules, rewrite_rules, user_agent, cookie, webhook_url, checked_at, hide_globally)
select n, case when n = 12 then 43 else 42 end,
    case when n = 10 then 8 when n = 12 then 9 else 7 end,
    'Additional fixture feed', 'https://example.test/feed/' || n, 'https://example.test/',
    '', '', '', '', '', '', now(), n = 11
from generate_series(10, 12) n;

insert into entries (id, user_id, feed_id, hash, published_at, title, url,
    changed_at, content, author, comments_url, status, score,
    raw_relevance_score, vote, starred, saved_for_later, share_code, tags, document_vectors)
select 200 + n, case when n = 20 then 43 else 42 end,
    case when n = 20 then 12 when n % 7 = 0 then 10 when n % 6 = 0 then 11 else 9 end,
    md5(n::text), now() - (n / 2) * interval '1 hour', 'Counted fixture article ' || n,
    'https://example.test/counted/' || n, now() - n * interval '1 minute',
    'Counted fixture body', '', '',
    case when n % 3 = 0 then 'read'::entry_status else 'unread'::entry_status end,
    n % 4, case when n % 5 = 0 then null else n * 2.5 end,
    n % 3 - 1, n % 2 = 0, n % 3 = 1,
    case when n % 2 = 0 then 'fixture-share-' || n else '' end,
    array['fixture'], to_tsvector('needle counted article')
from generate_series(1, 20) n;

insert into entry_user_tags (entry_id, user_tag_id)
values (201, 17), (203, 17), (208, 17);

insert into enclosures (id, user_id, entry_id, url, size, mime_type)
values (1, 42, 201, 'https://example.test/audio.mp3', 128, 'audio/mpeg');
