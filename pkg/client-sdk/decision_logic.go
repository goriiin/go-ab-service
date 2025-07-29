package client_sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/hashicorp/go-version"
)

// evaluateExperiment выполняет полную проверку одного эксперимента для пользователя.
func (c *Client) evaluateExperiment(ctx *DecisionContext, exp *ab_types.Experiment) (bool, string) {
	// 1. Предварительная фильтрация: эксперимент неактивен или завершен.
	if exp.Status != ab_types.StatusActive || (exp.EndTime != nil && exp.EndTime.Before(time.Now())) {
		return false, ""
	}

	// 2. Проверка ручных исключений: пользователь в списке ForceExclude.
	if slices.Contains(exp.OverrideLists.ForceExclude, ctx.UserID) {
		return false, ""
	}

	// 3. Проверка принудительного назначения варианта (имеет высший приоритет).
	if exp.OverrideLists.ForceInclude != nil {
		for variantName, userList := range exp.OverrideLists.ForceInclude {
			if slices.Contains(userList, ctx.UserID) {
				// Пользователь принудительно назначен этому варианту.
				// Логика таргетинга и бакетирования игнорируется.
				return true, variantName
			}
		}
	}

	// 4. Проверка правил таргетинга: пользователь должен соответствовать всем правилам.
	if !c.checkTargetingRules(ctx, exp.TargetingRules) {
		return false, ""
	}

	// 5. Финальное распределение (бакетирование).
	return c.getVariantForUser(ctx, exp)
}

// checkTargetingRules проверяет, удовлетворяет ли пользователь ВСЕМ правилам таргетинга.
func (c *Client) checkTargetingRules(ctx *DecisionContext, rules []ab_types.TargetingRule) bool {
	for _, rule := range rules {
		if !c.evaluateRule(ctx, &rule) {
			return false // Если хотя бы одно правило не выполнено, пользователь не подходит
		}
	}
	return true
}

// getVariantForUser вычисляет хеш и находит вариант для пользователя.
func (c *Client) getVariantForUser(ctx *DecisionContext, exp *ab_types.Experiment) (bool, string) {
	// Используем xxhash для максимальной производительности
	hashKey := []byte(ctx.UserID + exp.Salt)
	bucket := xxhash.Sum64(hashKey) % 1000

	for _, variant := range exp.Variants {
		if bucket >= uint64(variant.BucketRange[0]) && bucket <= uint64(variant.BucketRange[1]) {
			// Успешное назначение
			c.metrics.decisions.WithLabelValues(exp.ID, variant.Name).Inc()

			go c.trackAssignment(ctx, exp.ID, variant.Name)

			return true, variant.Name
		}
	}

	return false, ""
}

func (c *Client) trackAssignment(ctx *DecisionContext, expID, variantName string) {
	event := AssignmentEvent{
		UserID:       ctx.UserID,
		ExperimentID: expID,
		VariantName:  variantName,
		Timestamp:    time.Now().UTC(),
		Context:      ctx.Attributes,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("ERROR: Failed to marshal assignment event: %v", err)
		c.metrics.errors.WithLabelValues("assignment_marshal_error").Inc()
		return
	}

	// Отправляем с фоновым контекстом, чтобы не блокироваться
	err = c.assignmentProducer.Publish(context.Background(), []byte(ctx.UserID), payload)
	if err != nil {
		log.Printf("ERROR: Failed to publish assignment event to Kafka: %v", err)
		c.metrics.errors.WithLabelValues("assignment_publish_error").Inc()
	}
}

func (c *Client) evaluateRule(ctx *DecisionContext, rule *ab_types.TargetingRule) bool {
	userValue, ok := ctx.Attributes[rule.Attribute]
	if !ok {
		return false
	}

	switch rule.Operator {
	case ab_types.OpEquals:
		return fmt.Sprintf("%v", userValue) == fmt.Sprintf("%v", rule.Value)
	case ab_types.OpGreaterThan:
		userNum, ok1 := toFloat64(userValue)
		ruleNum, ok2 := toFloat64(rule.Value)
		if !ok1 || !ok2 {
			return false
		}
		return userNum > ruleNum
	case ab_types.OpInList:
		userStr := fmt.Sprintf("%v", userValue)
		ruleList, ok2 := rule.Value.([]interface{})
		if !ok2 {
			return false
		}
		for _, item := range ruleList {
			if strItem, ok := item.(string); ok && strItem == userStr {
				return true
			}
		}
		return false
	case ab_types.OpVersionGreaterThan:
		userVerStr, ok1 := userValue.(string)
		ruleVerStr, ok2 := rule.Value.(string)
		if !ok1 || !ok2 {
			return false
		}
		userV, err1 := version.NewVersion(userVerStr)
		ruleV, err2 := version.NewVersion(ruleVerStr)
		if err1 != nil || err2 != nil {
			return false
		}
		return userV.GreaterThan(ruleV)
	default:
		log.Printf("WARN: Unknown operator used: %s", rule.Operator)
		return false
	}
}

// toFloat64 - безопасный конвертер в float64 из разных числовых типов.
func toFloat64(v any) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case float32:
		return float64(i), true
	case int64:
		return float64(i), true
	case int32:
		return float64(i), true
	case int:
		return float64(i), true
	case string:
		f, err := strconv.ParseFloat(i, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
