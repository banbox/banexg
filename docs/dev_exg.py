from aibaton import run, set_default, start_process, setup_logger, logger
import os
import subprocess

'''
这是banexg快速对接新交易所的agent工作流脚本。

【使用流程】
## 如何快速对接新交易所
1. 安装crawl4ai，使用docs/crawler.py爬取交易所文档（请修改其中的url配置）。
2. 注册交易所账号，生成apiKey & apiSecret，维护到交易所目录的local.json中。
3. 安装codex/claude code，然后运行此脚本（修改其中的exg_name变量等）

【agent工作流会更快吗？】
不会，此工作流耗时是人工使用codex对接的大概2倍。token消耗3倍以上。
因为人工对接时关注其中重点信息，指令更明确，没有不必要的冗余步骤。
而工作流为了通用性，划分的任务较细，流程略长，agent交互次数更多。

【agent工作流的好处是？】
需要人工参与的时间更少，通过定制流程减少错误，让对banexg不太了解的人也可快速对接新交易所
'''

base_dir = os.path.dirname(os.path.abspath(__file__))
root_dir = os.path.realpath(os.path.join(base_dir, "../.."))
banexg_dir = os.path.realpath(os.path.join(root_dir, "banexg"))

setup_logger(filepath="aibaton.log")
set_default(provider="codex", dangerous_permissions=True, cwd=os.path.dirname(base_dir))

exg_name = 'bybit'
exg_doc = 'bybit_v5'
exg_dir = os.path.join(banexg_dir, exg_name)

gen_doc_index = f"""
请帮我创建 docs/{exg_name}_index.md 文件，这个文件应该用于描述 docs/{exg_doc} 下的所有文件的路径和每个文件的简单单行介绍。
用于AI通过此文件快速定位所需功能的路径。请使用最少的文本简要概括每个markdown文件的主要作用。
{exg_doc}下的每个文件只需读取前20~40行左右即可，每个文件都是一个接口，了解其大概作用即可。
最后创建{exg_name}_index.md ，减少冗余的文字，每个文件都要介绍到。应当分批进行，阅读一批，更新一次文件，然后继续阅读下一批。
"""

pick_plan_step = f"""
@docs/help.md @docs/contribute.md  @docs/{exg_name}_dev.md  @docs/{exg_name}_index.md 
目前需要对接{exg_name}交易所，实现banexg中需要的相关接口。请根据{exg_name}_dev.md 这个实施计划，帮我挑选下一步需要实现的部分。
在响应的末尾以<option>这是要实现的部分标题</option>格式输出。如果所有部分都已完成，则不要输出<option>部分。
"""

run_plan_step = f"""
@docs/help.md @docs/contribute.md  @docs/{exg_name}_dev.md  @docs/{exg_name}_index.md 
目前需要对接{exg_name}交易所，实现banexg中需要的相关接口。请根据{exg_name}_dev.md 这个详细的实施计划，帮我开始逐步对接{exg_name}交易所。
根据banexg的已有接口规范和币安、okx的参考，从{exg_name}_index中查找需要的接口，实现接口时根据接口路径从docs/{exg_doc} 下阅读详细文档。
对接过程中始终遵循DRY准则，检查是否有冗余或相似代码，有则提取公共部分，方便维护。
确保始终遵循banexg的规范要求，和根结构体的相关规范，如果有几个交易所共同的逻辑，则提取到外部公共包的代码文件中。
现在需要对接的部分是：{{section}}
"""

run_plan_check = f"""
@contribute.md @help.md @docs/{exg_name}_dev.md @docs/{exg_name}_index.md 
目前正在对接{exg_name}，目前已完成 {{section}} 部分，需要检查是否实现有错误或不完善的地方。
请阅读banexg接口和参数要求，适当参考binance中的处理，了解哪些参数和逻辑需要处理。
然后根据{exg_name}_index定位接口文件路径，阅读docs/{exg_doc}下的详细接口文档。
注意一些常见重要的参数都需要支持，但部分不常用的，交易所特有的参数无需支持。可参考binance/okx等接口相关方法。
最后把发现的需要修改或完善的地方总结给我。如果此部分的实现均正确且无缺漏，则在响应最后输出<promise>DONE</promise>。
请注意只关注 {{section}} 部分。
"""

code_refactor = """
发现冗余代码时，提取为子函数，确保遵循DRY原则，减少重复或相似的代码片段；
* 核心原则是尽量减少冗余或相似代码逻辑，方便维护。
* 保持业务逻辑不变。保持样式整体不变可细微调整。
* 当某些部分可能和其他文件中的某些重合时，考虑提取公共部分复用；
* 如果某函数body只有一行且参数不超过3个，则应该删除，在引用地方直接改为简短代码。
* 对于大部分相似但细微不同的，提取为带参数的可复用函数、组件或片段
"""

run_code_refactor = """
使用`git status -s`查看当前修改的文件，重点对这些文件进行代码审查并优化。
""" + code_refactor

code_merge = """
@docs/help.md 
现在整体go文件数量过多，上面这些文件行数过少，应当将其中相近的进行合并，减少文件数量。
* 合并后行数在270~800行为宜。
* 功能相近的优先合并到一起（创建新文件），并重新命名。请勿合并到行数已经过多的文件中。
* 应当将被合并的文件对应的_test.go文件也对应进行合并，test.go文件行数不限。
* 对确实不相干，但内容较少的文件等，应当合并到common/util等后缀的文件中。
"""

run_plan_test = f"""
@docs/contribute.md @docs/help.md @docs/{exg_name}_dev.md @docs/{exg_name}_index.md 
目前正在对接{exg_name}，目前已完成 {{section}} 部分，现在需要对这部分完善单元测试用例并确保测试通过。
单元测试需要两类：一类是简单的函数测试（不发出接口请求）；另一类是实际提交到交易所的接口测试（统一使用`TestApi_`前缀）。可参考binance中的相关单元测试；
然后根据{exg_name}_index定位接口文件路径，阅读docs/{exg_doc}下的详细接口文档。
首先确保第一类测试完整并全部通过，如果有错误自行分析解决，重复测试直到通过。
然后开始第二类测试，这些测试应该使用local.json中配置的apiKey和apiSecret创建一个有效的交易所对象，然后调用实际的接口方法和交易所生产环境接口进行交互。
第二类测试有些需要提前有仓位，可以先执行某个单元测试下单创建仓位，然后测试相关的接口。
请注意只关注 {{section}} 部分。如果确信测试全部通过且均无缺漏和错误，则在响应最后输出<promise>DONE</promise>。
"""

run_plan_mark = f"""
@docs/{exg_name}_dev.md 请帮我把此文档中 {{section}} 部分的实现标记为完成
"""

update_help = """
@docs/help.md 这是banexg项目的索引文件，目前项目代码已经进行了很多更新，此索引文件中有不少是过时的；请你以文件夹为单位，逐个文件夹、逐个代码文件，分析查看其包含的主要逻辑和功能，然后更新到索引文件中。
务必分批次进行，读取一些，分析一些，更新一些；索引文件中不存在的描述进行删除，一些以实际代码逻辑为准；保持当前索引文件风格，简要凝练，无冗余信息，概括性高。
"""

logger.info('开始生成文档索引...')
run(gen_doc_index)

logger.info('开始生成实施计划...')
run(f"""
@docs/help.md @docs/contribute.md  @docs/{exg_doc}  @docs/{exg_name}_index.md 
目前需要对接{exg_name}交易所，实现banexg中需要的相关接口。
请先阅读help.md和contribute.md，了解banexg的架构和实现规范。明确需要实现的接口。
然后阅读{exg_name}_index.md 了解{exg_name}提供的所有接口；
然后随意挑选7个{exg_doc}下的接口文档阅读，了解{exg_name}接口参数和返回数据的格式特点。明确需要如何处理接口数据解析。
目前主要有两种：binance是接口返回数据完全不同，直接每个接口定义结构体解析即可；okx是接口返回的数据有部分一致，比如全都嵌套在data字段中，这种可以通过泛型传入不同部分，减少代码冗余。
然后根据banexg中的其他要求，自行从{exg_name}文档中阅读所需信息概要。
最后综合所有信息，制定一份实施计划；此计划中禁止大段代码，以简洁凝练的风格逐步描述分阶段的实施步骤，但任务粒度要足够小尽可能详细。
注意banexg中涉及的所有交易所接口和各种参数都需要实现，尽可能从接口文档中找到对应的，整理到{exg_name}_dev中
各个部分的预估耗时应该接近。应该按互相之间依赖关系按顺序排列。已完成的部分标记为完成。
输出计划内容到docs/{exg_name}_dev.md
""")

def run_plan_steps():
    while True:
        logger.info('选择下一个要处理的计划步骤...')
        pick_res = run(pick_plan_step)
        section = pick_res.select()
        if not section:
            break
        for i in range(1):
            logger.info(f'运行计划步骤: {section}')
            run(run_plan_step.format(section=section))
            logger.info(f'检查计划步骤: {section}')
            check_res = run(run_plan_check.format(section=section))
            if check_res.select("promise") == "DONE":
                break
        
        logger.info(f'运行代码重构优化: {section}')
        run(run_code_refactor)
        logger.info(f'运行单元测试: {section}')
        run(run_plan_test.format(section=section), loop_max=5)
        logger.info(f'标记计划完成: {section}')
        run(run_plan_mark.format(section=section))
        logger.info(f'更新help: {section}')
        run(update_help)

# 第一次按计划逐步实施
run_plan_steps()

logger.info('开始整体检查是否接口有错误...')
run(f"""
@docs/help.md @docs/contribute.md  @docs/{exg_name}_dev.md @docs/{exg_doc}  @docs/{exg_name}_index.md 
目前正在对接{exg_name}交易所，现在已初步实现大部分必要的接口。但仍可能有很多潜在问题或bug或遗漏。
请先阅读help.md和contribute.md，了解banexg的架构和实现规范。明确需要实现的接口。
然后阅读{exg_name}_dev.md 了解初步的实施计划方案。再阅读{exg_name}_index.md 了解{exg_name}提供的所有接口；
注意banexg中涉及的所有交易所接口和各种参数都需要实现，尽可能从接口文档中找到对应的，整理到{exg_name}_dev中
然后以banexg接口为单位，逐个根据参数，适当参考binance中的参数实现；再阅读{exg_name}中涉及的相关接口文档，对于必要的所有参数都需要支持，检查是否有遗漏或错误。
将发现的所有错误或遗漏等更新到{exg_name}_dev.md中，简述即可，不需要详细代码描述。把需要修改的部分状态改为待完成。
""")

# 针对第二次发现的问题，重新逐步实施
run_plan_steps()

logger.info('设置工作目录为: ' + root_dir)
set_default(cwd=root_dir)

logger.info('修改测试yaml...')
run(f"""
banbot/go.mod
banstrats/go.mod
请帮我在上面2个项目的mod文件中，对依赖的banexg和banbot启用replace指令，确保直接使用本地代码编译。
data/config.local.yml
然后在这个yaml配置中，确保进行如下修改：
```yaml
env: prod
market_type: linear
time_start: "20250101"
time_end: "20260101"
put_limit_secs: 300
stake_amount: 10
leverage: 10
pairs: ['XRP']
run_policy:
  - name: tmp:limit_order
    run_timeframes: [1m]
exchange:
  name: {exg_name}
```
然后帮我在banstrats下执行`go build -o bot`编译，然后启动一个单独的可视化终端（使用gnome-terminal）异步持续运行`./bot spider`。
""")

strat_dir = os.path.realpath(os.path.join(root_dir, "banstrats"))


def get_ban_exchange_methods():
    '读取 intf.go 中 BanExchange 接口的所有函数名'
    import re
    intf_path = os.path.join(base_dir, "../intf.go")
    with open(intf_path, "r", encoding="utf-8") as f:
        content = f.read()
    # 提取 BanExchange 接口定义块
    match = re.search(r'type BanExchange interface \{([\s\S]*?)\n\}', content)
    if not match:
        return []
    block = match.group(1)
    # 匹配函数名：行首的标识符后跟(
    methods = re.findall(r'^\s*(\w+)\s*\(', block, re.MULTILINE)
    return methods

# 测试驱动开发，每个接口都生成充分的测试用例执行检查
prg_path = os.path.join(banexg_dir, "docs/tdd_methods.md")
if not os.path.exists(prg_path):
    with open(prg_path, "r", encoding="utf-8") as f:
        tdd_methods = f.read()
else:
    tdd_methods = ""
methods = get_ban_exchange_methods()
finishes = {"LoadMarkets", "GetCurMarkets", "GetMarket", "MapMarket", "InitLeverageBrackets", "CheckSymbols", "Info", 
"SetFees", "Call", "SetDump", "SetReplay", "GetReplayTo", "ReplayOne", "ReplayAll", "SetOnWsChan", "PrecAmount", 
"PrecPrice", "PrecCost", "PrecFee", "HasApi", "SetOnHost", "PriceOnePip", "IsContract", "MilliSeconds", "GetAccount", 
"SetMarketType", "GetExg", "Close", "GetNetDisable",}
finishes.update(re.findall(r'\b([_a-zA-Z0-9]+)\b', tdd_methods, re.MULTILINE))
for method in methods:
    if method in finishes:
        continue
    logger.info(f"正在TDD处理方法: {method}")
    run(f"""
@docs/contribute.md @docs/help.md @docs/{exg_name}_dev.md @docs/{exg_name}_index.md 
目前已经完成了{exg_name}交易所的对接，不过接口单元测试还不够全面。请检查{exg_name}包下的 {method} 方法，根据其中涉及的{exg_name}接口，查阅对应的{exg_name}接口文档。
然后根据当前banexg支持的参数情况，适当参考binance/okx的实现，了解每一个接口的可能的参数组合有哪些。
根据每种参数组合，都单独生成一个测试用例，确保测试用例能够覆盖到所有可能的参数组合。(比如下单接口市价单、触发单所需参数不同；只是参数值不一样的应视为一个用例)
这些测试用例应该直接发出HTTP请求，所以都以`TestApi_`前缀开头。如果不需要新增单元测试，则直接结束。
生成的单元测试必须放在方法所在文件对应的_test.go文件中。如method在exg.go中，则放在exg_test.go中。
执行单元测试前，先执行命令清空仓位和未成交挂单：
```bash
cd {banexg_dir}
go clean -cache
go test -v -run TestApi_CloseAllPositions ./{exg_name}/
go test -v -run TestApi_CancelAllOpenOrders ./{exg_name}/
```
注意测试用例代码也需要遵循DRY原则，发现冗余代码时，提取为子函数，减少重复或相似的代码片段；
然后逐个执行测试新增的单元测试用例，确保全部通过。如果有问题，则检查测试用例是否正确，或者此参数是否应该受支持，如果都正确，则检查banexg的接口实现是否有错误并修复。
修复后重新运行测试用例，确保全部通过。完成后将当前方法写入到docs/tdd_methods.md中，只新增一行记录方法名即可""")


def close_positions():
    # 清理并关闭所有仓位和订单
    logger.info('清理Go缓存...')
    subprocess.run("go clean -cache", shell=True, cwd=banexg_dir)
    logger.info('关闭所有仓位...')
    subprocess.run(f"go test -v -run TestApi_CloseAllPositions ./{exg_name}/", shell=True, cwd=banexg_dir)
    logger.info('取消所有未完成订单...')
    subprocess.run(f"go test -v -run TestApi_CancelAllOpenOrders ./{exg_name}/", shell=True, cwd=banexg_dir)

def get_fix_bug(source: str, path: str):
    tpl = f"""
banbot/doc/help.md
banexg/docs/help.md
banexg/docs/{exg_name}_index.md
{{holder}}
请阅读相关banbot和banexg中{exg_name}的相关代码。帮我分析解决；
如果需要{exg_name}接口文档，先阅读{exg_name}_index定位接口文件路径，阅读docs/{exg_doc}下的详细接口文档。
深入思考，了解根本原因并解决。只需修复代码即可，可允许单元测试和编译测试，禁止重新执行实盘。
如果日志符合预期没有异常，无需修复，则在响应最后输出<promise>DONE</promise>。
"""
    hold_text = f"""
{path}
banstrats/trade.log 这是banexg新对接{exg_name}在banbot中的实盘测试日志。
日志中预期应该有一个下单记录，然后晚些有订单成交日志。如果不符合预期，则可能有bug。"""
    if source == "backtest":
        hold_text = f"""
{path}
banstrats/backtest.log 这是banexg新对接{exg_name}在banbot中的量化回测日志。
日志中预期应该有很多订单，在最后的统计中BarNum应该至少>100，如果不符合预期，则可能有bug。"""
    elif source == "compile":
        hold_text = f"""
{path}
这是banbot编译错误日志，请根据相关代码帮我排查修复。修复后可在banstrats下执行`go build -o bot`编译测试。"""
    return tpl.format(holder=hold_text)

# 运行实盘测试并自动修复bug
def run_and_fix(source: str, path: str):
    fix_tip = get_fix_bug(source, path)
    for i in range(20):
        # compile
        res = start_process("go build -o bot", cwd=strat_dir, timeout_s=360).watch(stream=True)
        if res.returncode != 0:
            logger.error(f'compile failed: {res.output}')
            run(get_fix_bug("compile", res.output))
            continue
        if source == "trade":
            close_positions()
        # run
        res = start_process("./bot "+source, cwd=strat_dir, timeout_s=360).watch(stream=True)
        with open(os.path.join(strat_dir, f"{source}.log"), "w", encoding="utf-8") as f:
            f.write(res.output)
        logger.info(f'[{i+1}/20] run and fix bug...')
        fix_res = run(fix_tip)
        if fix_res.select("promise") == "DONE":
            break

def set_yaml_policy(name: str):
    return f"""
@banbot/doc/config.yml @banbot/doc/help.md 先阅读这两个文件，然后帮我把 data/config.local.yml 中的 run_policy 进行如下修改。务必遵循config.yml的格式：
run_policy:
 - name: {name}
   run_timeframes: [1m]"""

# 运行限价单测试
logger.info('运行限价单测试...')
run(set_yaml_policy("tmp:limit"))
run_and_fix("trade", "banstrats/tmp/limit_order.go")
# 运行触发入场测试
logger.info('运行触发入场测试...')
run(set_yaml_policy("tmp:trigger"))
run_and_fix("trade", "banstrats/tmp/trigger_ent.go")


# 运行双均线回测
logger.info('运行双均线回测...')
run(set_yaml_policy("ma:demo"))
run_and_fix("backtest", "banstrats/ma/demo.go")


def get_go_files(root_dir: str, min_lines: int = 0, max_lines: int = 0) -> list[str]:
    go_files = []
    for dirpath, _, filenames in os.walk(root_dir):
        for f in filenames:
            if f.endswith('.go'):
                fpath = os.path.join(dirpath, f)
                line_count = sum(1 for _ in open(fpath))
                if (min_lines <= 0 or line_count > min_lines) and (max_lines <= 0 or line_count < max_lines):
                    go_files.append(fpath)
    return go_files

# 拆分过长的代码文件
file_paths = sorted(get_go_files(exg_dir, min_lines=1100))
for i, fpath in enumerate(file_paths):
    line_count = sum(1 for _ in open(fpath))
    logger.info(f"拆分过长代码文件 [{i+1}/{len(file_paths)}] {fpath} ({line_count}行)")
    out_num = line_count // 500
    run(f"""{fpath}
当前文件有点长，帮我拆分为 {out_num} 个文件，把相似功能的函数放在一起，确保拆分后行数大致相同；
注意分批编辑，不然会因一次性输出太长而失败。一次大概处理200行即可。
注意直接复制相关代码，先创建后删除，确保文件中函数全部迁移后才删除文件。
按函数名搜索单元测试，把单元测试也严格按照新拆分后的文件名_test.go进行组织""", cwd=banexg_dir)

# 重构冗长的代码文件
file_paths = sorted(get_go_files(exg_dir, min_lines=300))
for i, fpath in enumerate(file_paths):
    logger.info(f"代码优化重构 [{i+1}/{len(file_paths)}] {fpath}")
    run(f"@docs/help.md {fpath}\n\n{code_refactor}", cwd=banexg_dir)

# 合并过短的代码文件
file_paths = sorted(get_go_files(exg_dir, max_lines=170))
file_paths = [path for path in file_paths if not path.endswith("_test.go")]
logger.info(f"合并过短代码文件 {len(file_paths)} 个")
path_text = '\n'.join(file_paths)
run(f"{path_text}{code_merge}", cwd=banexg_dir)
