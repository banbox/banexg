@contribute.md 这是当前banexg项目的描述文档。此文档旨在用尽可能少但准确的语言概况描述banexg的架构、核心接口、风格、原则和规范、实现细节等。目的在于让阅读此文件的AI能快速理解项目，在新的模块开发任务中编写符合项目规范的代码。请你再次阅读当前项目所有重要的代码文件。根据上面原则对此文件进行修改，要求减少无用信息（如优势之类），对信息密度过低的地方进行凝练，确保用简短语言描述重要内容，对描述不足的重要信息进行补充。

## 实现交易所
@docs/help.md @docs/contribute.md  @docs/okx_dev.md  @docs/okx_api_index.md 
目前需要对接okx交易所，实现banexg中需要的相关接口。请根据ookx_dev.md 这个详细的实施计划，帮我开始逐步对接okx交易所。
务必分批次逐步小步迭代，完成一部分测试一部分，注意多用单元测试。
根据banexg的已有接口规范和币安、bybit的参考，从okx_api_index中查找需要的接口，实现接口时根据索引范围从docs/okx.md阅读一些接口详细文档。
okx.md 有4万行，禁止整体阅读，只能根据okx_api_index中接口行号范围针对性阅读。
对接过程中始终遵循DRY准则，完成一部分工作后，就检查是否有冗余或相似代码，有则提取公共部分，方便维护。
确保始终遵循banexg的规范要求，和根结构体的相关规范，如果有几个交易所共同的逻辑，则提取到外部公共包的代码文件中。

## 生成真实接口测试
@docs/contribute.md @docs/help.md docs/okx_api_index.md 
当前已初步实现okx交易所对接，不过在单元测试方面还有些问题。
单元测试除了简单的非接口测试，还需要补充实际提交到交易所的接口测试。可参考binance中的相关单元测试；
如果需要okx接口文档，先阅读okx_api_index了解行号范围，然后再根据范围阅读 banexg/docs/okx.md 
这些测试应该使用local.json中配置的apiKey和apiSecret创建一个有效的交易所对象，然后调用实际的接口方法和交易所生产环境接口进行交互。
这些测试统一使用`TestApi_`前缀，在自动批量测试时应被排除，只应由用户手动单个执行这些测试。

## 检查实现是否正确
@contribute.md @help.md docs/okx_api_index.md 
目前okx已基本支持完毕，不过有不少细节可能未正确实现。
请阅读banexg接口和参数要求，适当参考binance中的处理，了解哪些参数和逻辑需要处理。
如果需要okx接口文档，先阅读okx_api_index了解行号范围，然后再根据范围阅读 docs/okx.md 
最后整理输出到docs/okx_dev.md 中。
对于找不到对应接口但缺失的逻辑，也需要维护在okx_dev中。

## bug自动修复
[日志] + [问题描述]
banbot/doc/help.md
banexg/docs/help.md
banexg/docs/okx_api_index.md
上面是banbot启动实盘的日志，可能出现了一些错误。banexg对已支持的binance正常，这是刚对接okx的实盘测试日志。
请阅读相关banbot和banexg中okx的相关代码。帮我分析解决；如果需要okx接口文档，先阅读okx_api_index了解行号范围，然后再根据范围阅读 banexg/docs/okx.md 
深入思考，了解根本原因并解决。