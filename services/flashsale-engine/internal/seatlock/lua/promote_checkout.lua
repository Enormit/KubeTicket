-- Promote HELD seats owned by userID to CHECKOUT:<orderID> with extended TTL.
-- KEYS[1..N]   = seat:lock:<eventID>:<seatID>
-- KEYS[N+1]    = seat:holds_zset:<eventID>
-- KEYS[N+2]    = seat:seq:<eventID>
-- ARGV[1]      = userID
-- ARGV[2]      = checkoutLockValue ("CHECKOUT|<userID>|<orderID>|<expMs>")
-- ARGV[3]      = checkoutTTLSeconds
-- ARGV[4]      = expMs
-- ARGV[5..4+N] = seatIDs

local n = #KEYS - 2
local expectedPrefix = "HELD|" .. ARGV[1] .. "|"

for i = 1, n do
    local cur = redis.call("GET", KEYS[i])
    if cur == false or string.sub(cur, 1, #expectedPrefix) ~= expectedPrefix then
        return {0, ARGV[4 + i]}
    end
end

local ttl = tonumber(ARGV[3])
local expMs = tonumber(ARGV[4])
for i = 1, n do
    redis.call("SET", KEYS[i], ARGV[2], "EX", ttl)
    redis.call("ZADD", KEYS[n + 1], expMs, ARGV[4 + i])
end

local seq = redis.call("INCR", KEYS[n + 2])
return {1, tostring(seq)}
