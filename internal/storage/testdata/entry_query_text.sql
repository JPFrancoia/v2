create temporary table users (like public.users including defaults);
create temporary table categories (like public.categories including defaults);
create temporary table feeds (like public.feeds including defaults);
create temporary table entries (like public.entries including defaults);
create temporary table icons (like public.icons including defaults);
create temporary table feed_icons (like public.feed_icons including defaults);

insert into users (id, username, timezone) values (42, 'query_text_fixture', 'UTC');
insert into categories (id, user_id, title) values (7, 42, 'Fixture category');
insert into feeds (id, user_id, category_id, title, feed_url, site_url,
    description, scraper_rules, rewrite_rules, user_agent, cookie, webhook_url, checked_at)
values (9, 42, 7, 'Fixture feed', 'https://example.test/feed', 'https://example.test/',
    '', '', '', '', '', '', now());
insert into entries (id, user_id, feed_id, hash, published_at, title, url,
    changed_at, content, one_line_summary, author, comments_url, status, score,
    tags, document_vectors)
select 100+n, 42, 9, md5(n::text), now(), 'Fixture article',
    'https://example.test/article/' || n, now(), repeat('Original article body. ', 400),
    'Stored generated summary.', '', '', 'unread', 55, '{}', to_tsvector('needle article')
from generate_series(1, 3) n;
