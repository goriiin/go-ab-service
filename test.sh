#!/bin/sh

# --- Конфигурация ---
API_HOST="http://central-api:8080"
SORT_APP_HOST="http://example-sort-app:8081"
EXPERIMENT_ID=""

# --- Функция: Ожидание готовности сервиса ---
wait_for_service() {
    local url=$1
    local method=${2:-GET}
    local body=${3:-''}
    local description=$4

    echo "--- Ожидание готовности: ${description} (${url}) ---"
    for i in $(seq 1 15); do
        if [ "$method" = "POST" ]; then
            if curl -s --fail-with-body -X POST -H "Content-Type: application/json" -d "$body" -o /dev/null "$url"; then
                echo "Сервис ${description} готов."
                return 0
            fi
        else
            if curl -s -f -o /dev/null "$url"; then
                echo "Сервис ${description} готов."
                return 0
            fi
        fi
        echo "Попытка ${i}/15: Сервис недоступен, повтор через 2 секунды..."
        sleep 2
    done
    echo "ОШИБКА: Сервис ${description} не ответил в течение 30 секунд."
    exit 1
}

# --- Основной сценарий ---

# Ожидание готовности базовых сервисов
wait_for_service "${API_HOST}/health" "GET" "" "Central API"
wait_for_service "${SORT_APP_HOST}/sort" "POST" '{"user_id":"healthcheck","numbers":[]}' "Sort App"

# ШАГ 1: Создание эксперимента в статусе DRAFT
echo "\n--- Создание эксперимента для сортировки (статус DRAFT) ---"
CREATE_PAYLOAD='{
    "layer_id": "sorting_layer",
    "targeting_rules": [{"attribute": "use_sort_test", "operator": "EQUALS", "value": true}],
    "variants": [
        {"name": "variant-a-asc", "bucket_range": [0, 499]},
        {"name": "variant-b-desc", "bucket_range": [500, 999]}
    ]
}'
RESPONSE_BODY=$(curl -s -X POST ${API_HOST}/experiments -H "Content-Type: application/json" -d "$CREATE_PAYLOAD")
EXPERIMENT_ID=$(echo "$RESPONSE_BODY" | jq -r .id)
if [ -z "$EXPERIMENT_ID" ] || [ "$EXPERIMENT_ID" = "null" ]; then
    echo "ОШИБКА: Не удалось создать эксперимент. Ответ API:"
    echo "$RESPONSE_BODY"
    exit 1
fi
echo "Эксперимент создан с ID: ${EXPERIMENT_ID}"

# ШАГ 2: Активация эксперимента и добавление оверрайдов через PUT
echo "\n--- Активация эксперимента и добавление оверрайдов (статус ACTIVE) ---"
ACTIVATE_PAYLOAD='{
    "layer_id": "sorting_layer",
    "status": "ACTIVE",
    "targeting_rules": [{"attribute": "use_sort_test", "operator": "EQUALS", "value": true}],
    "variants": [
        {"name": "variant-a-asc", "bucket_range": [0, 499]},
        {"name": "variant-b-desc", "bucket_range": [500, 999]}
    ],
    "override_lists": {
        "force_include": {
            "variant-a-asc": ["user-for-asc"],
            "variant-b-desc": ["user-for-desc"]
        }
    }
}'
curl -s -f -X PUT ${API_HOST}/experiments/${EXPERIMENT_ID} -H "Content-Type: application/json" -d "$ACTIVATE_PAYLOAD" -o /dev/null
echo "Эксперимент ${EXPERIMENT_ID} активирован."

# КРИТИЧЕСКИ ВАЖНО: Пауза для асинхронного распространения конфигурации через Kafka
echo "\n--- Ожидание распространения конфигурации в SDK (15 секунд) ---"
sleep 15

echo "\n--- Проверка работы example-sort-app ---"

echo "Проверка варианта 'variant-a-asc' для 'user-for-asc'..."
SORT_RESPONSE_ASC=$(curl -s -X POST ${SORT_APP_HOST}/sort \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-for-asc", "numbers": [5,1,4,2,3]}')

VARIANT_ASC=$(echo "$SORT_RESPONSE_ASC" | jq -r .variant_used)
SORTED_NUMS_ASC=$(echo "$SORT_RESPONSE_ASC" | jq -r .sorted_numbers | tr -d ' \n')

if [ "$VARIANT_ASC" != "variant-a-asc" ] || [ "$SORTED_NUMS_ASC" != "[1,2,3,4,5]" ]; then
    echo "ОШИБКА! Неверный результат для варианта ASC:"
    echo "$SORT_RESPONSE_ASC"
    exit 1
fi
echo "УСПЕХ: Вариант 'variant-a-asc' отработал корректно."

echo "Проверка варианта 'variant-b-desc' для 'user-for-desc'..."
SORT_RESPONSE_DESC=$(curl -s -X POST ${SORT_APP_HOST}/sort \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-for-desc", "numbers": [5,1,4,2,3]}')

VARIANT_DESC=$(echo "$SORT_RESPONSE_DESC" | jq -r .variant_used)
SORTED_NUMS_DESC=$(echo "$SORT_RESPONSE_DESC" | jq -r .sorted_numbers | tr -d ' \n')

if [ "$VARIANT_DESC" != "variant-b-desc" ] || [ "$SORTED_NUMS_DESC" != "[5,4,3,2,1]" ]; then
    echo "ОШИБКА! Неверный результат для варианта DESC:"
    echo "$SORT_RESPONSE_DESC"
    exit 1
fi
echo "УСПЕХ: Вариант 'variant-b-desc' отработал корректно."

echo "\n--- Удаление эксперимента ---"
curl -s -f -X DELETE ${API_HOST}/experiments/${EXPERIMENT_ID}
echo "Эксперимент ${EXPERIMENT_ID} удален."

echo "\n--- ВСЕ ТЕСТЫ ПРОЙДЕНЫ УСПЕШНО ---"
exit 0