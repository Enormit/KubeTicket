-- Safely expire a seat lock from ZSET if its stored timestamp is <= nowMs.
-- KEYS[1] = seat:lock:<eventID>:<seatID>
-- KEYS[2] = seat:holds_zset:<eventID>
-- KEYS[3] = seat:seq:<eventID>
-- ARGV[1] = nowMs
-- ARGV[2] = seatID

local cur = redis.call("GET", KEYS[1])
if cur == false then
    redis.call("ZREM", KEYS[2], ARGV[2])
    local seq = redis.call("INCR", KEYS[3])
    return {1, tostring(seq)}
end

if string.sub(cur, 1, 5) == "SOLD|" then
    redis.call("ZREM", KEYS[2], ARGV[2])
    return {0, "0"}
end

local lastPipe = string.match(cur, ".*|(%d+)$")
local expMs = tonumber(lastPipe or "0")
if expMs <= tonumber(ARGV[1]) then
    redis.call("DEL", KEYS[1])
    redis.call("ZREM", KEYS[2], ARGV[2])
    local seq = redis.call("INCR", KEYS[3])
    return {1, tostring(seq)}
end

return {0, "0"}
