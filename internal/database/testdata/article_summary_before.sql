create temporary table entries (
    id bigint primary key,
    content text not null,
    one_line_summary text not null default ''
);
insert into entries values (1, 'Original article body.', 'Retired summary.');
