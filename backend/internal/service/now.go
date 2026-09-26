package service

import "time"

// nowFunc 当前时间，便于测试注入（handler 层另有同名函数，依赖方向保持 service 不依赖 handler）。
var nowFunc = time.Now
