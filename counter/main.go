package main

import (
	"fmt"
	"github.com/go-redis/redis"
	"log"
	"sync"
)

const luascript = `
-- 获取调用脚本时传入的第一个 key 值（用作限流的 key）
local key = KEYS[1]
-- 获取调用脚本时传入的第一个参数值（限流大小）
local limit = tonumber(ARGV[1])
-- 获取计数器的限速区间 TTL
local ttl = tonumber(ARGV[2])

-- 获取当前流量大小
local currentLimit = tonumber(redis.call('GET', key) or "0")

-- 是否超出限流
if (currentLimit >= limit) then
    -- 返回 (拒绝)
    return 0
end

-- 如果 key 中保存的并发计数为 0，说明当前是一个新的时间窗口，它的过期时间设置为窗口的过期时间
if (currentLimit == 0) then
    redis.call('SETEX', key, ttl, 1)
else
	-- 那就在原来的计数上+1
	redis.call('INCR', key)	
end

-- 返回 (放行)
return 1
`

func main() {
	var wg sync.WaitGroup
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	for i := 0; i <= 10; i++ {
		wg.Add(1)
		go evalScript(client, i, &wg)
	}
	wg.Wait()
}

func evalScript(client *redis.Client, index int, wg *sync.WaitGroup) {
	defer wg.Done()
	script := redis.NewScript(luascript)
	sha, err := script.Load(client).Result()
	if err != nil {
		log.Fatalf("Load Script Err: %s\n", err.Error())
	}
	result, err := client.EvalSha(sha, []string{"counter-limit"}, 1, 60).Result()
	if err != nil {
		log.Fatalf("Exec Lua Err: %s\n", err.Error())
	}

	fmt.Printf("请求: %d 是否能通过 %d\n", index, result)
}

// ❯ go run counter/main.go
// 请求: 8 是否能通过 1
// 请求: 4 是否能通过 0
// 请求: 10 是否能通过 0
// 请求: 2 是否能通过 0
// 请求: 9 是否能通过 0
// 请求: 7 是否能通过 0
// 请求: 0 是否能通过 0
// 请求: 6 是否能通过 0
// 请求: 3 是否能通过 0
// 请求: 5 是否能通过 0
// 请求: 1 是否能通过 0
