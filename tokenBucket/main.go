package main

import (
	"fmt"
	"github.com/go-redis/redis"
	"log"
	"sync"
	"time"
)

const luascript = `
-- key
local key = KEYS[1]
-- 最大存储的令牌数
local max_permits = tonumber(KEYS[2])
-- 每秒钟产生的令牌数
local permits_per_second = tonumber(KEYS[3])

-- 如果令牌桶不存在，则先创建
local exist = tonumber(redis.call("EXISTS", key))
if exist == 0 then
	redis.call("HMSET", key, "last_ticket_timestamp", 0, "stored_permits", 0)
end

-- 上次拿走令牌的时间 
local last_ticket_timestamp = tonumber(redis.call('hget', key, 'last_ticket_timestamp'))
-- 当前剩余的令牌数
local stored_permits = tonumber(redis.call('hget', key, 'stored_permits'))

-- 当前时间
local time = redis.call('time')
-- time[1] 返回的为 UNIX 秒级时间戳
local now_timestamp = tonumber(time[1]) 

-- 还有多的令牌，返回通过
if stored_permits > 1 then
	redis.call("HMSET", key, "stored_permits", stored_permits-1)
	return 1
end

-- 当前时间小于上次拿走令牌时间，且令牌已经是0个，返回不通过
if now_timestamp <= last_ticket_timestamp then
	return 0
end

-- 每秒能生成多少个令牌
local stable_interval = 1 / permits_per_second
-- 计算这段时间需要新生成多少个令牌
local new_permits = (now_timestamp - last_ticket_timestamp) * stable_interval
-- 如果补充的令牌超过容量，取最小的那个
stored_permits = math.min(max_permits, stored_permits + new_permits)

-- 补充完毕之后还是没有令牌（两次获取令牌时间太短）
if stored_permits < 1 then
	return 0
end

-- 补充到桶里
redis.call("HMSET", key, "last_ticket_timestamp", now_timestamp, "stored_permits", stored_permits-1)
return 1
`

func main() {
	var wg sync.WaitGroup
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// 每秒钟生成一个令牌，最大存放10个

	// 先拿5个，此时还剩5个
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go evalScript(client, i, &wg)
	}

	// 休息 1 秒，还剩 6 个
	time.Sleep(1 * time.Second)
	fmt.Println("sleep 1s")
	// 再拿 10 个，应该有 4 个拿不到
	for i := 0; i < 10; i++ {
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
	result, err := client.EvalSha(sha, []string{"token-limit", "10", "1"}).Result()
	if err != nil {
		log.Fatalf("Exec Lua Err: %s\n", err.Error())
	}

	fmt.Printf("请求: %d 需要等待 %d\n", index, result)
}

// ❯ go run tokenBucket/main.go
// 请求: 1 需要等待 1
// 请求: 4 需要等待 1
// 请求: 2 需要等待 1
// 请求: 0 需要等待 1
// 请求: 3 需要等待 1
// sleep 1s
// 请求: 2 需要等待 1
// 请求: 1 需要等待 1
// 请求: 0 需要等待 1
// 请求: 3 需要等待 1
// 请求: 9 需要等待 1
// 请求: 5 需要等待 0
// 请求: 4 需要等待 0
// 请求: 7 需要等待 0
// 请求: 6 需要等待 0
// 请求: 8 需要等待 0
