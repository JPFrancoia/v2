with weeks as (
	select generate_series(
		date_trunc('week', current_timestamp at time zone $2) - interval '11 weeks',
		date_trunc('week', current_timestamp at time zone $2),
		interval '1 week'
	)::date as week_start
)
select
	weeks.week_start,
	count(entries.id) filter (
		where entries.vote >= 0 and (entries.starred or entries.vote = 1)
	),
	count(entries.id),
	count(entries.id) filter (where entries_importance.is_important),
	count(entries.id) filter (where entries_importance.is_important and entries.vote = 1),
	count(entries.id) filter (where entries_importance.is_important is false and entries.vote = 1)
from weeks
left join entries on
	entries.user_id = $1
	and entries.status = 'read'
	and entries.changed_at >= weeks.week_start::timestamp at time zone $2
	and entries.changed_at < (weeks.week_start::timestamp + interval '1 week') at time zone $2
left join entries_importance on entries_importance.entry_id = entries.id
group by weeks.week_start
order by weeks.week_start desc
