# GitHub 发布与维护

项目使用 Git 管理，主分支为 main。远程仓库为 https://github.com/Blessing-BOS/campus-events ，提交作者使用仓库所有者确认的身份。

## 准备好的内容

- 完整源码、go.mod / go.sum、SQL、页面、截图、README、接口与验收记录。
- Dockerfile、Compose 和 GitHub Actions。
- .gitignore 已排除 .run、.env、构建产物和本地密钥；.env.example 只包含占位配置。

## 首次发布流程

1. 登录 GitHub，创建空仓库并确定公开或私有。
2. 确认提交作者名称与邮箱。
3. 选择是否为自己的业务代码添加许可证；第三方依赖的原许可继续有效。
4. 检查待提交文件，确认没有 .run、.env、实际密码、数据库文件和二进制。
5. 创建初始提交、关联远程并推送。
6. 等待 CI；修复真实环境暴露的问题后，再把链接放到简历和主页。

## 后续更新

在项目根目录修改并验证代码后：

```text
git status
git add .
git commit -m "Describe the change"
git push
```

GitHub 的 Actions 页面可以查看 CI 结果。源码仓库与运行中的网站是两件事：上传源码不会自动部署 Go 服务。
