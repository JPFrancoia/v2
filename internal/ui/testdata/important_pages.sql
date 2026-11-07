create temporary table users (like public.users including defaults);
create temporary table categories (like public.categories including defaults);
create temporary table feeds (like public.feeds including defaults);
create temporary table entries (like public.entries including defaults);
create temporary table icons (like public.icons including defaults);
create temporary table feed_icons (like public.feed_icons including defaults);
create temporary table user_tags (like public.user_tags including defaults);
create temporary table entry_user_tags (like public.entry_user_tags including defaults);
create temporary table enclosures (like public.enclosures including defaults);
create temporary table entries_importance (entry_id bigint primary key, is_important boolean not null);
insert into users (id, username, timezone, language, theme, entries_per_page, mark_read_on_view)
values (42, 'important_handler', 'UTC', 'en_US', 'light_serif', 2, true),
    (43, 'other_handler', 'UTC', 'en_US', 'light_serif', 2, true);
insert into categories (id,user_id,title,hide_globally) values (7,42,'Visible',false),(8,42,'Hidden',true),(9,43,'Other',false);
insert into feeds (id,user_id,category_id,title,feed_url,site_url,description,scraper_rules,rewrite_rules,user_agent,cookie,webhook_url,checked_at,hide_globally)
select n,case when n=12 then 43 else 42 end,case when n=10 then 8 when n=12 then 9 else 7 end,
    'Synthetic feed','https://example.test/feed/'||n,'https://example.test/','','','','','','',now(),n=11
from generate_series(9,12) n;
insert into entries (id,user_id,feed_id,hash,published_at,title,url,changed_at,content,author,comments_url,status,score,document_vectors)
select 100+n,case when n=8 then 43 else 42 end,case when n=6 then 10 when n=7 then 11 when n=8 then 12 else 9 end,
    md5(n::text),now()-n*interval '1 hour','Synthetic article '||n,'https://example.test/article/'||n,now(),'<p>Synthetic body</p>','','','unread',10,to_tsvector('synthetic')
from generate_series(1,8) n;
insert into entries_importance (entry_id,is_important) select id,id<>104 from entries where id<>105;
