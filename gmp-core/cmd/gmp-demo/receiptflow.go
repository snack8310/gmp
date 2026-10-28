package main

import (
	"fmt"
	"os"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

func showReceiptFlow(out *os.File, stack *scenarios.Stack) error {
	section(out, "活动流程 · 回执边",
		"同一个活动里，下一步由边上的回执结果决定，人取回执上的那一个。不是场景 ⑤：四小时和未读仍是两个活动加回流。也不是上面的实验臂：那里一个活动挂住各臂，没有边。")

	built, err := stack.ReceiptFlow()
	if err != nil {
		return err
	}
	if err := showReceiptSerial(out, stack, built.Serial); err != nil {
		return err
	}
	if err := showReceiptBranch(out, built.Branch); err != nil {
		return err
	}
	if err := showReceiptParallel(out, stack, built.Parallel); err != nil {
		return err
	}
	fmt.Fprintf(out, "  未表达：%s\n", built.Missing)
	return nil
}

func showReceiptSerial(out *os.File, stack *scenarios.Stack, serial scenarios.ReceiptSerial) error {
	fmt.Fprintf(out, "  串行 · 活动 %s\n", serial.Campaign.ID())
	fmt.Fprintf(out, "    入口")
	for _, treatment := range serial.Campaign.Entries() {
		fmt.Fprintf(out, " %s", treatment.ID())
	}
	fmt.Fprintln(out)

	_, err := serial.Runner.Run(serial.Campaign, serial.Next, "entry", audience.Live())
	if err != nil {
		fmt.Fprintf(out, "    对非入口 %s 批量跑：%v\n", serial.Next, err)
	} else {
		fmt.Fprintf(out, "    对非入口 %s 批量跑被接受了 —— 这是个缺陷\n", serial.Next)
	}

	report, err := serial.Runner.Run(serial.Campaign, serial.Entry, "entry", audience.Live())
	if err != nil {
		return err
	}
	var success, failure int
	for i, execution := range report.Executions {
		outcome := campaign.OutcomeSuccess
		if i%2 == 1 {
			outcome = campaign.OutcomeFailure
			failure++
		} else {
			success++
		}
		if err := serial.Runner.RecordReceipt(serial.Campaign, campaign.Receipt{
			Key: execution.Key, Arrived: true, Outcome: outcome,
		}); err != nil {
			return err
		}
	}
	mailN := effectsOf(serial.Deliverers, scenarios.RecallMailAction)
	fmt.Fprintf(out, "    入口投递 %d 人；回执成功 %d、失败 %d；下一步 %s 投递 %d 人\n",
		len(report.Executions), success, failure, serial.Next, mailN)

	next, ok := treatmentByID(serial.Campaign, serial.Next)
	if !ok {
		return fmt.Errorf("gmp-demo: serial campaign has no %s", serial.Next)
	}
	nextN, err := memberCount(stack, next.Audience())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    下一步自己的人群有 %d 人，下一步实际投递 %d 人\n", nextN, mailN)
	return nil
}

func showReceiptBranch(out *os.File, branch scenarios.ReceiptBranch) error {
	fmt.Fprintf(out, "  按回执分支 · 活动 %s\n", branch.Campaign.ID())
	report, err := branch.Runner.Run(branch.Campaign, branch.Entry, "entry", audience.Live())
	if err != nil {
		return err
	}
	var success, failure int
	for i, execution := range report.Executions {
		outcome := campaign.OutcomeSuccess
		if i%2 == 1 {
			outcome = campaign.OutcomeFailure
			failure++
		} else {
			success++
		}
		if err := branch.Runner.RecordReceipt(branch.Campaign, campaign.Receipt{
			Key: execution.Key, Arrived: true, Outcome: outcome,
		}); err != nil {
			return err
		}
	}
	mail := calledUIDs(branch.Deliverers[scenarios.BranchMailAction])
	sms := calledUIDs(branch.Deliverers[scenarios.BranchSMSAction])
	var cross int
	for uid := range mail {
		if sms[uid] {
			cross++
		}
	}
	fmt.Fprintf(out, "    入口 %d 人；成功 %d 人走 %s（实际 %d）；失败 %d 人走 %s（实际 %d）；交叉 %d\n",
		len(report.Executions), success, branch.OnSuccess, len(mail),
		failure, branch.OnFailure, len(sms), cross)
	return nil
}

func showReceiptParallel(out *os.File, stack *scenarios.Stack, parallel scenarios.ReceiptParallel) error {
	fmt.Fprintf(out, "  无边并行 · 活动 %s\n", parallel.Campaign.ID())
	fmt.Fprintf(out, "    入口")
	for _, treatment := range parallel.Campaign.Entries() {
		fmt.Fprintf(out, " %s", treatment.ID())
	}
	fmt.Fprintln(out)

	leftReport, err := parallel.Runner.Run(parallel.Campaign, parallel.Left, "entry", audience.Live())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    先跑 %s：%d 人；此时 %s 投递 %d 人\n",
		parallel.Left, len(leftReport.Executions), parallel.Right,
		effectsOf(parallel.Deliverers, scenarios.ParallelNoticeAction))

	if len(leftReport.Executions) > 0 {
		if err := parallel.Runner.RecordReceipt(parallel.Campaign, campaign.Receipt{
			Key: leftReport.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
		}); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "    给 %s 记一条成功回执后，%s 仍是 %d 人\n",
		parallel.Left, parallel.Right, effectsOf(parallel.Deliverers, scenarios.ParallelNoticeAction))

	rightReport, err := parallel.Runner.Run(parallel.Campaign, parallel.Right, "entry", audience.Live())
	if err != nil {
		return err
	}
	left, ok := treatmentByID(parallel.Campaign, parallel.Left)
	if !ok {
		return fmt.Errorf("gmp-demo: parallel campaign has no %s", parallel.Left)
	}
	right, ok := treatmentByID(parallel.Campaign, parallel.Right)
	if !ok {
		return fmt.Errorf("gmp-demo: parallel campaign has no %s", parallel.Right)
	}
	leftN, err := memberCount(stack, left.Audience())
	if err != nil {
		return err
	}
	rightN, err := memberCount(stack, right.Audience())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    再跑 %s：%d 人。两边执行 %d 与 %d，各自的人群 %d 与 %d\n",
		parallel.Right, len(rightReport.Executions),
		len(leftReport.Executions), len(rightReport.Executions), leftN, rightN)
	return nil
}

func treatmentByID(built campaign.Campaign, id campaign.TreatmentID) (campaign.Treatment, bool) {
	for _, treatment := range built.Treatments() {
		if treatment.ID() == id {
			return treatment, true
		}
	}
	return campaign.Treatment{}, false
}

func memberCount(stack *scenarios.Stack, definition audience.Definition) (int, error) {
	members, err := stack.Audiences.Enumerate(definition, audience.Live())
	if err != nil {
		return 0, err
	}
	return len(members), nil
}

func effectsOf(deliverers map[campaign.ActionID]*campaign.MemoryDeliverer, action campaign.ActionID) int {
	deliverer := deliverers[action]
	if deliverer == nil {
		return 0
	}
	return deliverer.Effects()
}

func calledUIDs(deliverer *campaign.MemoryDeliverer) map[audience.UID]bool {
	out := map[audience.UID]bool{}
	if deliverer == nil {
		return out
	}
	for _, call := range deliverer.Calls() {
		out[call.UID] = true
	}
	return out
}
