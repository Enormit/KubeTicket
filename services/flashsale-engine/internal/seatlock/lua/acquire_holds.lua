-- Acquire up to N seats atomically for a user.
-- KEYS[1..N]   = seat:lock:<eventID>:<seatID>
-- KEYS[N+1]    = seat:holds_zset:<eventID>
-- KEYS[N+2]    = seat:active_events
-- KEYS[N+3]    = seat:seq:<eventID>
-- ARGV[1]      = lockValue ("HELD|<userID>|<holdToken>|<expMs>")
-- ARGV[2]      = ttlSeconds
-- ARGV[3]      = expMs
-- ARGV[4]      = eventID
-- ARGV[5..4+N] = seatIDs

local n = #KEYS - 3
for i = 1, n do
    local cur = redis.call("GET", KEYS[i])
    if cur ~= false then
        return {0, ARGV[4 + i]}
    end
end

local ttl = tonumber(ARGV[2])
local expMs = tonumber(ARGV[3])
for i = 1, n do
    redis.call("SET", KEYS[i], ARGV[1], "EX", ttl)
    redis.call("ZADD", KEYS[n + 1], expMs, ARGV[4 + i])
end

redis.call("SADD", KEYS[n + 2], ARGV[4])
local seq = redis.call("INCR", KEYS[n + 3])
return {1, tostring(seq)}
