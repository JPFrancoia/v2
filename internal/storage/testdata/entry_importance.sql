create temporary table entries_importance (entry_id bigint primary key, is_important boolean not null);
insert into entries_importance (entry_id, is_important)
select id, id % 2 = 0 from entries;
