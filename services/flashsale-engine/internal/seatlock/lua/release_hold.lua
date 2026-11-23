-- Release a HELD seat if owned by the requesting user.
-- KEYS[1] = seat:lock:<eventID>:<seatID>
-- KEYS[2] = seat:holds_zset:<eventID>
-- KEYS[3] = seat:seq:<eventID>
-- ARGV[1] = userID
-- ARGV[2] = seatID

local cur = redis.call("GET", KEYS[1])
if cur == false then
    redis.call("ZREM", KEYS[2], ARGV[2])
    return {0, "not_held"}
end

local expectedPrefix = "HELD|" .. ARGV[1] .. "|"
if string.sub(cur, 1, #expectedPrefix) ~= expectedPrefix then
    return {-1, "forbidden"}
end

redis.call("DEL", KEYS[1])
redis.call("ZREM", KEYS[2], ARGV[2])
local seq = redis.call("INCR", KEYS[3])
return {1, tostring(seq)}
