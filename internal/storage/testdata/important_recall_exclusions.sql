with cohort as (
    select date_trunc('week', current_timestamp at time zone 'America/Los_Angeles') - interval '1 week' as week_start
), samples (id,user_id,status,vote,starred,is_important) as (
    values (215,42,'read',1,false,null), (216,42,'unread',1,false,false),
        (217,42,'removed',1,false,false), (218,42,'read',0,true,false),
        (219,42,'read',-1,false,false), (220,43,'read',1,false,false)
), inserted as (
    insert into entries (id,user_id,feed_id,hash,published_at,title,url,changed_at,content,author,comments_url,status,vote,starred)
    select id,user_id,9,md5(id::text),now(),'Synthetic recall exclusion','https://example.test/recall/'||id,
        (week_start + interval '2 days') at time zone 'America/Los_Angeles','','','',status::entry_status,vote,starred
    from samples cross join cohort returning id
)
insert into entries_importance (entry_id,is_important)
select id,is_important from samples join inserted using (id) where is_important is not null;
