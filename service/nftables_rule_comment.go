package service

import "strings"

type nftRuleCommentProfile struct {
	prefix string
}

func (p nftRuleCommentProfile) base(tag string) string {
	return p.prefix + strings.TrimSpace(tag)
}

func (p nftRuleCommentProfile) in(tag string) string {
	return p.base(tag) + "_in"
}

func (p nftRuleCommentProfile) out(tag string) string {
	return p.base(tag) + "_out"
}

func (p nftRuleCommentProfile) redirect(tag string) string {
	return p.base(tag) + "_redirect"
}

func (p nftRuleCommentProfile) forward(tag string) string {
	return p.base(tag) + "_forward"
}

var (
	singboxNftRuleComments      = nftRuleCommentProfile{prefix: "kwor_inbound_"}
	mihomoNftRuleComments       = nftRuleCommentProfile{prefix: "kwor_mihomo_inbound_"}
	singboxLimitNftRuleComments = nftRuleCommentProfile{prefix: "kwor_client_limit_"}
	mihomoLimitNftRuleComments  = nftRuleCommentProfile{prefix: "kwor_mihomo_client_limit_"}
	singboxBlockNftRuleComments = nftRuleCommentProfile{prefix: "kwor_client_block_"}
	mihomoBlockNftRuleComments  = nftRuleCommentProfile{prefix: "kwor_mihomo_client_block_"}
	trafficCapNftRuleComments   = nftRuleCommentProfile{prefix: "kwor_traffic_cap_"}
)
