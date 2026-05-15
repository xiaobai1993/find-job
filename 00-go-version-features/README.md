# Go 1.18 - 1.23 各版本重要特性总结

## 📁 文件说明

| 文件 | 说明 |
|------|------|
| `go-1.18-to-1.23-features.md` | 完整的版本特性文档 |
| `examples/go1.18-generics.go` | Go 1.18 泛型示例 |
| `examples/go1.21-oncevalue.go` | Go 1.21 OnceValue/slog/min/max 示例 |
| `examples/go1.22-loopvar.go` | Go 1.22 循环变量修复示例 |
| `examples/go1.23-iterator.go` | Go 1.23 迭代器示例 |

## 🚀 运行示例

```bash
# Go 1.21+ 示例（当前版本可用）
go run examples/go1.21-oncevalue.go

# Go 1.22+ 示例（当前版本可用）
go run examples/go1.22-loopvar.go

# Go 1.23+ 示例（需要升级 Go 版本）
# go run examples/go1.23-iterator.go
```

## 📊 当前 Go 版本检查

```bash
go version
# go version go1.22.12 darwin/arm64
```

## 🎯 面试重点回顾

| 版本 | 必背特性 | 面试频率 |
|------|---------|---------|
| **Go 1.22** | 循环变量修复、range over int | ⭐⭐⭐⭐⭐ |
| **Go 1.18** | 泛型、模糊测试 | ⭐⭐⭐⭐⭐ |
| **Go 1.21** | OnceFunc/OnceValue、slog、min/max | ⭐⭐⭐⭐⭐ |
| **Go 1.19** | 类型化原子操作 | ⭐⭐⭐⭐ |
| **Go 1.23** | 迭代器、unique 包 | ⭐⭐⭐⭐ |

## 💡 升级建议

- **新项目**：直接用 Go 1.22+，避免循环变量坑
- **老项目**：尽快升级到 Go 1.21+，使用 OnceValue/slog
- **面试准备**：重点掌握 Go 1.18/1.21/1.22 特性

---

**参考来源：**
- https://go.dev/doc/go1.18
- https://go.dev/doc/go1.19
- https://go.dev/doc/go1.20
- https://go.dev/doc/go1.21
- https://go.dev/doc/go1.22
- https://go.dev/doc/go1.23
