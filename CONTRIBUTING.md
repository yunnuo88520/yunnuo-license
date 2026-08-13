# 参与贡献

感谢你参与允诺云授权系统。提交较大的功能前，建议先创建 Issue 说明使用场景、边界和方案，
以减少重复工作。

## 开发流程

1. Fork 仓库并从 `main` 创建功能分支。
2. 保持改动聚焦，不提交数据库、缓存、构建产物或真实密钥。
3. 为行为变更补充测试和必要文档。
4. 运行相关测试，确认通过后提交 Pull Request。

完整检查命令：

```bash
(cd backend && gofmt -w . && go test ./...)
(cd sdk/go && gofmt -w . && go test ./...)
(cd sdk/node && npm test && npm run check)
(cd sdk/java && mvn -B test)
```

涉及部署、数据库迁移或前后端集成的改动还应运行：

```bash
./docker/test-image.sh yunnuo-license:test
```

## Pull Request

Pull Request 请说明改动原因、主要行为、测试结果和兼容性影响。涉及界面改动时附上桌面端与
移动端截图；涉及数据库结构时说明升级与回滚方式。

提交即表示你同意按本项目的 Apache License 2.0 授权你的贡献。
