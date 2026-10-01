//go:build !race

package main

// 非竞态插桩构建标记：默认构建下参与编译，是分配预算的正常判定环境。
// 竞态插桩下的同名常量见 race_on_test.go。
var isRaceEnabled = false
