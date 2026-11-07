select count(id), count(id) filter (where vote >= 0 and (starred or vote = 1))
from entries where user_id = $1 and status = 'read'
    and changed_at >= $2::date::timestamp at time zone $3
    and changed_at < ($2::date::timestamp + interval '1 week') at time zone $3;
