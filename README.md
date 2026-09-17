# QuarkLangQkfmt

QuarkLang 代码格式化器（正典语法）——工具链成员，独立项目、零依赖。

## 用法

```sh
qkfmt [files...]        # 打印格式化结果到 stdout
qkfmt -w [files...]     # 原地写回（保持文件权限）
qkfmt -l [files...]     # 只列出未格式化的文件（CI 用；有差异退出码 1）
qkfmt -d [files...]     # 打印逐行差异
```

安装：`go build -o qkfmt .`（Go 1.21+）。

## 规则（gofmt 风格，克制）

- 4 空格缩进（按 `{}` 深度）；一行一条语句
- `{` 与前项同行留一空格；`}` 独占一行，后接 `else`/`catch` 时同行
- 二元运算符两侧一空格；一元运算符紧贴（`-a`、`!b`、`*l`）
- `,` 后一空格；`(`/`[` 紧贴前项（`if`/`while`/`for`/`catch`/`return` 例外，留一空格）
- `.`/`::` 两侧不留空格
- 保留注释（含行尾注释与块注释原文）；折叠连续空行为 1；清除行尾空白
- 文件末尾恰好一个换行

**只改空白，不增删任何 token**（语义不变）；输出**幂等**（两遍格式化结果字节一致）。

## 正典语法

格式化器按 QuarkLang 正典语法输出（类型在前声明、`fn<T> name(int a) Ret`、`type struct … Name;`、
`impl<T> { … } Name;`、`space { … } name;`、`for (int x : l)`、`catch (void e)`、运算符方法名
`__add__` 等）。完整清单见主仓 `SYNTAX.md`。

## CI 接入

```yaml
- name: 检查格式
  run: qkfmt -l $(git ls-files '*.qk')
```

## 实测

| 仓库 | .qk 文件 | 未格式化 |
|---|---|---|
| QuarkLang（主仓 + 示例 + testdata） | 10 | 7 |
| QuarkLangLibs-Style | 5 | 5 |
| QuarkLangLibs-Cleg | 11 | 11 |
| QuarkLangLibs-Regex | 3 | 3 |
| QuarkLangLibs-Json | 1 | 1 |
| QuarkLangLibs-Actions | 1 | 1 |

12 个源文件抽样：token 差异 0、幂等失败 0；格式化后的 `regex.qk` 测试 `failures=0`、
`cleg.qk` 仍能 parse+typecheck（仅报预期的 cannot run a library）。
