// Command gmp-demo walks the business scenarios over the in-memory model and
// prints what happens.
//
// It exists to make the model visible, not to test it: the regression cases
// live in the scenarios package and run under go test. Nothing here reads
// configuration, listens on anything, or writes to disk.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

const population = 1000

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "gmp-demo: %v\n", err)
		os.Exit(1)
	}
}

func run(out *os.File) error {
	stack, err := scenarios.NewStack(population)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "抽象模型已装配：%d 人的样本，来源 %q，三层全部走内存实现\n", len(stack.People), scenarios.SourceID)
	fmt.Fprintln(out, "不接数据库、不接渠道、不起服务。以下全部是模型算出来的真实结果。")

	if err := showBannerContest(out, stack); err != nil {
		return err
	}
	if err := showRollout(out, stack); err != nil {
		return err
	}
	if err := showArms(out, stack); err != nil {
		return err
	}
	if err := showDelivery(out, stack); err != nil {
		return err
	}
	if err := showTwoStep(out); err != nil {
		return err
	}
	// A fresh stack: the sections above have already coloured this
	// population, and a store that is already full would make the real run
	// look as though it colours nobody either.
	if err := showRehearsal(out); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n未覆盖：场景 ① 的「未使用」来自券系统，属另一个来源；一份人群定义能否跨多个来源仍是开放问题。")
	return nil
}

func section(out *os.File, title, note string) {
	fmt.Fprintf(out, "\n── %s ──\n%s\n\n", title, note)
}

func showBannerContest(out *os.File, stack *scenarios.Stack) error {
	section(out, "场景 ③ · APP 首页 banner",
		"大促（优先级 100）与会员召回（优先级 50）抢同一个位子；大促内部用决策表分新客 / 老客。")

	contest, err := stack.BannerContest()
	if err != nil {
		return err
	}
	counts := map[string]int{}
	shown := 0
	for _, person := range stack.People {
		decision, err := contest.Campaigns.Decide(scenarios.BannerSlot, person.UID, audience.Live())
		if err != nil {
			return err
		}
		if decision.Won {
			counts[string(decision.Award.Campaign)+" / "+string(decision.Award.Treatment)]++
		} else {
			counts["无人接手"]++
		}
		if shown < 3 {
			shown++
			printOne(out, person, decision)
		}
	}
	fmt.Fprintln(out, "  全样本落点：")
	for _, line := range sorted(counts) {
		fmt.Fprintf(out, "    %s\n", line)
	}
	return nil
}

func printOne(out *os.File, person audience.Record, decision campaign.Decision) {
	tier := person.Attributes["tier"]
	if decision.Won {
		image, _ := decision.Award.Call.Param("image")
		fmt.Fprintf(out, "  %s（%s）→ %s / %s，素材 %s\n",
			person.UID, tier, decision.Award.Campaign, decision.Award.Treatment, image)
	} else {
		fmt.Fprintf(out, "  %s（%s）→ 什么都不给\n", person.UID, tier)
	}
	for _, loss := range decision.Losses {
		fmt.Fprintf(out, "      留痕：%s / %s 未中，原因：%s\n", loss.Campaign, loss.Treatment, why(loss.Reason))
	}
}

func showRollout(out *os.File, stack *scenarios.Stack) error {
	section(out, "场景 ④ · 灰度放量",
		"切 100 份，逐步纳入。关键不是纳入了多少，是已经进来的人一个都不掉。")

	var previous map[audience.UID]bool
	for _, step := range []int{5, 20, 100} {
		service, err := stack.RolloutAdmitting(step)
		if err != nil {
			return err
		}
		current := map[audience.UID]bool{}
		for _, person := range stack.People {
			decision, err := service.Decide(scenarios.BannerSlot, person.UID, audience.Live())
			if err != nil {
				return err
			}
			if decision.Won {
				current[person.UID] = true
			}
		}
		dropped := 0
		for uid := range previous {
			if !current[uid] {
				dropped++
			}
		}
		fmt.Fprintf(out, "  放到前 %d 份：纳入 %d 人", step, len(current))
		if previous == nil {
			fmt.Fprintln(out, "")
		} else {
			fmt.Fprintf(out, "，上一步的人掉出去 %d 个\n", dropped)
		}
		previous = current
	}
	fmt.Fprintln(out, "  用「随机取 5%」的话，第二次取的跟第一次不是同一批人，而且不报错。")
	return nil
}

func showArms(out *os.File, stack *scenarios.Stack) error {
	section(out, "场景 ② · 双十一分组实验",
		"近 30 天有加购的人切四份。对照组挂零个投放项，但必须在系统里存在。")

	arms, err := stack.ExperimentArms()
	if err != nil {
		return err
	}
	base, err := stack.Audiences.Enumerate(arms.Base, audience.Live())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  基础人群：%d 人\n", len(base))
	covered := 0
	for _, name := range arms.Order {
		members, err := stack.Audiences.Enumerate(arms.Arms[name], audience.Live())
		if err != nil {
			return err
		}
		covered += len(members)
		note := ""
		if name == arms.Control {
			note = "  ← 对照组，挂 0 个投放项"
		}
		fmt.Fprintf(out, "    %-18s %d 人%s\n", name, len(members), note)
	}
	fmt.Fprintf(out, "  四臂合计 %d 人，与基础人群相差 %d —— 不重不漏\n", covered, covered-len(base))
	return nil
}

func showRehearsal(out *os.File) error {
	section(out, "演练 · 不能有副作用",
		"演练走「只读问一下」：算份号但不写染色记录，否则演练会改变它要预览的东西。")

	stack, err := scenarios.NewStack(population)
	if err != nil {
		return err
	}
	before := stack.Colouring.Size()
	arms, err := stack.ExperimentArms()
	if err != nil {
		return err
	}
	if _, err = stack.Audiences.Enumerate(arms.Arms[arms.Control], audience.Rehearsal()); err != nil {
		return err
	}
	afterRehearsal := stack.Colouring.Size()
	if _, err := stack.Audiences.Enumerate(arms.Arms[arms.Control], audience.Live()); err != nil {
		return err
	}
	fmt.Fprintf(out, "  染色记录：演练前 %d，演练后 %d，真实跑一遍后 %d\n",
		before, afterRehearsal, stack.Colouring.Size())
	return nil
}

// why renders a loss reason for a human reading the demo. The reasons
// themselves stay in the domain's own vocabulary; this is presentation.
func showTwoStep(out *os.File) error {
	section(out, "场景 ⑤ · 两级触达，只靠事件相连",
		"第一步发 WhatsApp，四小时后对未读的人发短信。两个投放项互不相识——第二步的人群定义建在回流来源上。")

	// A fresh stack: this chain registers its own backflow source.
	stack, err := scenarios.NewStack(population)
	if err != nil {
		return err
	}
	chain, err := stack.CrossBorderWinBack()
	if err != nil {
		return err
	}

	first, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Live())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  第一步：发出 %d 条 WhatsApp\n", len(first.Executions))

	var unread int
	for i, execution := range first.Executions {
		status := "yes"
		if i%2 != 0 {
			status = "no"
			unread++
		}
		if err := chain.Runner.RecordReceipt(campaign.Receipt{
			Key: execution.Key, Arrived: true, Attributes: map[string]string{"read": status},
		}); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "  渠道回执：其中 %d 人未读（「未读」是渠道自己的说法，平台不解释）\n", unread)

	tooSoon, err := chain.Runner.Run(chain.Second, chain.SecondStep, "follow-up", audience.Live())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  还没到四小时就跑第二步：触达 %d 人\n", len(tooSoon.Executions))

	chain.Backflow.Advance(4 * time.Hour)
	fmt.Fprintln(out, "  回流来源把时间推进四小时（时间归来源，核心从不问现在几点）")

	second, err := chain.Runner.Run(chain.Second, chain.SecondStep, "follow-up", audience.Live())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  第二步：对 %d 人发短信 —— 正是未读的那批\n", len(second.Executions))
	fmt.Fprintln(out, "  第二步的配置里没有任何指向第一步的引用，只有一个建在回流来源上的人群定义。")
	return nil
}

func why(reason campaign.LossReason) string {
	switch reason {
	case campaign.LossOutsideRollout:
		return "不在当前放量范围内"
	case campaign.LossAudienceDidNotMatch:
		return "人群不命中"
	case campaign.LossRoutedElsewhere:
		return "决策表把他路由到别处"
	case campaign.LossOnPriority:
		return "优先级输了"
	default:
		return string(reason)
	}
}

func showDelivery(out *os.File, stack *scenarios.Stack) error {
	section(out, "执行侧 · 幂等键贯穿三段",
		"决策 → 投递 → 回执，三段带同一个键。断一节，那次执行就永远挂着，且不报错。")

	delivery, err := stack.ArmDelivery(2)
	if err != nil {
		return err
	}
	// Learn the keys without sending, then make one of them fail every attempt.
	planned, err := delivery.Runner.Run(delivery.Campaign, delivery.Treatment, "entry", audience.Rehearsal())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  演练：打算投递 %d 人，执行记录 %d 条（演练不落记录）\n",
		len(planned.Planned), delivery.Log.Size())
	if len(planned.Planned) == 0 {
		return nil
	}
	doomed := planned.Planned[0].Key
	delivery.Deliverer.FailNext(doomed, 99)

	report, err := delivery.Runner.Run(delivery.Campaign, delivery.Treatment, "entry", audience.Live())
	if err != nil {
		return err
	}
	var failed int
	for _, execution := range report.Executions {
		if execution.State == campaign.StateFailed {
			failed++
		}
	}
	fmt.Fprintf(out, "  真实跑：投递 %d 人，其中 %d 人重试耗尽仍失败\n", len(report.Executions), failed)

	// Pick one the downstream actually took: a receipt saying something
	// arrived, for a delivery that was never accepted, would be nonsense.
	var sample campaign.Execution
	for _, execution := range report.Executions {
		if execution.State == campaign.StateDelivered {
			sample = execution
			break
		}
	}
	if sample.Key == "" {
		fmt.Fprintln(out, "  没有一条被下游接收，回执这一段跳过")
		return nil
	}
	fmt.Fprintf(out, "  取一条被接收的看三段：\n")
	fmt.Fprintf(out, "    决策产出 → 投放项 %s，落点 %s\n", sample.Treatment, sample.Placements[0])
	fmt.Fprintf(out, "    投递携带 → 幂等键 %q，尝试 %d 次\n", sample.Key, sample.Attempts)
	if err := delivery.Runner.RecordReceipt(campaign.Receipt{Key: sample.Key, Arrived: true, ExternalRef: "carrier-8891"}); err != nil {
		return err
	}
	closed, _, err := delivery.Log.Lookup(sample.Key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    回执带同一个键回来 → 状态 %s，外部标识 %s\n", closed.State, closed.ExternalRef)

	if err := delivery.Runner.RecordReceipt(campaign.Receipt{Key: "一个从没投递过的键", Arrived: true}); err != nil {
		fmt.Fprintf(out, "  对不上的回执被拒绝：%v\n", err)
	} else {
		fmt.Fprintln(out, "  对不上的回执被接受了 —— 这是个缺陷")
	}

	before := delivery.Deliverer.Effects()
	if _, err := delivery.Runner.Run(delivery.Campaign, delivery.Treatment, "entry", audience.Live()); err != nil {
		return err
	}
	fmt.Fprintf(out, "  同一次进入重跑一遍：实际投递次数 %d → %d（键相同，下游认得出来）\n",
		before, delivery.Deliverer.Effects())
	fmt.Fprintf(out, "  失败的那 %d 人仍带着自己的份号入账 —— 剔掉他们等于偷偷筛人群\n", failed)
	return nil
}

func sorted(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for key, value := range counts {
		out = append(out, fmt.Sprintf("%-40s %d 人", key, value))
	}
	sort.Strings(out)
	return out
}
