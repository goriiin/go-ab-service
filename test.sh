set -e # Прерывать выполнение при любой ошибке

# --- Переменные ---
API_HOST="http://central-api:8080"
SORT_APP_HOST="http://example-sort-app:8081"
EXPERIMENT_ID=""
JSON_PAYLOAD_CREATE='{
    "layer_id": "sorting_layer",
    "targeting_rules": [
        { "attribute": "use_sort_test", "operator": "EQUALS", "value": true }
    ],
    "variants": [
        { "name": "variant-a-asc", "bucket_range": [0, 499] },
        { "name": "variant-b-desc", "bucket_range": [500, 999] }
    ]
}'

echo "--- 1. Создание эксперимента ---"
RESPONSE_BODY=$(curl -s -X POST ${API_HOST}/experiments \
  -H "Content-Type: application/json" \
  -d "$JSON_PAYLOAD_CREATE")

EXPERIMENT_ID=$(echo "$RESPONSE_BODY" | jq -r .id)
if [ -z "$EXPERIMENT_ID" ] || [ "$EXPERIMENT_ID" = "null" ]; then
    echo "Ошибка: Не удалось получить EXPERIMENT_ID. Ответ:"
    echo "$RESPONSE_BODY"
    exit 1
fi
echo "Эксперимент создан с ID: ${EXPERIMENT_ID}"

echo "\n--- 2. Активация эксперимента ---"
JSON_PAYLOAD_ACTIVATE=$(echo "$JSON_PAYLOAD_CREATE" | jq '. + {"status": "ACTIVE"}')

curl -s -f -X PUT ${API_HOST}/experiments/${EXPERIMENT_ID} \
  -H "Content-Type: application/json" \
  -d "$JSON_PAYLOAD_ACTIVATE" > /dev/null
echo "Эксперимент ${EXPERIMENT_ID} активирован."

# --- НАЧАЛО ИЗМЕНЕНИЙ: Надежное ожидание обновления SDK ---
echo "\n--- 3. Ожидание обновления конфигурации в SDK (до 30 секунд) ---"
ATTEMPTS=0
MAX_ATTEMPTS=30
VARIANT="default"

while [ "$VARIANT" = "default" ] && [ $ATTEMPTS -lt $MAX_ATTEMPTS ]; do
    echo "Попытка #${ATTEMPTS}: Проверка варианта..."

    POLL_RESPONSE=$(curl -s -X POST ${SORT_APP_HOST}/sort \
      -H "Content-Type: application/json" \
      -d '{"user_id": "user-asc", "numbers": [1]}')

    VARIANT=$(echo "$POLL_RESPONSE" | jq -r .variant_used)

    if [ "$VARIANT" = "default" ]; then
        sleep 1
        ATTEMPTS=$((ATTEMPTS + 1))
    else
        echo "Конфигурация SDK обновлена! Получен вариант: ${VARIANT}"
    fi
done

if [ "$VARIANT" = "default" ]; then
    echo "Ошибка: SDK не обновил конфигурацию за ${MAX_ATTEMPTS} секунд."
    exit 1
fi
# --- КОНЕЦ ИЗМЕНЕНИЙ ---

echo "\n--- 4. Тестирование A/B вариантов ---"

echo "Проверка варианта 'variant-a-asc'..."
SORT_RESPONSE_ASC=$(curl -s -X POST ${SORT_APP_HOST}/sort \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-asc", "numbers": [5,1,4,2,3]}')

VARIANT_ASC=$(echo "$SORT_RESPONSE_ASC" | jq -r .variant_used)
SORTED_NUMS_ASC=$(echo "$SORT_RESPONSE_ASC" | jq .sorted_numbers | tr -d '\n ')

if [ "$VARIANT_ASC" != "variant-a-asc" ] || [ "$SORTED_NUMS_ASC" != "[1,2,3,4,5]" ]; then
    echo "Ошибка! Неверный результат для варианта ASC:"
    echo "$SORT_RESPONSE_ASC"
    exit 1
fi
echo "Вариант ASC - Успех."

echo "Проверка варианта 'variant-b-desc'..."
SORT_RESPONSE_DESC=$(curl -s -X POST ${SORT_APP_HOST}/sort \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-desc", "numbers": [5,1,4,2,3]}')

VARIANT_DESC=$(echo "$SORT_RESPONSE_DESC" | jq -r .variant_used)
SORTED_NUMS_DESC=$(echo "$SORT_RESPONSE_DESC" | jq .sorted_numbers | tr -d '\n ')

if [ "$VARIANT_DESC" != "variant-b-desc" ] || [ "$SORTED_NUMS_DESC" != "[5,4,3,2,1]" ]; then
    echo "Ошибка! Неверный результат для варианта DESC:"
    echo "$SORT_RESPONSE_DESC"
    exit 1
fi
echo "Вариант DESC - Успех."

echo "\n--- 5. Удаление эксперимента для очистки ---"
curl -s -f -X DELETE ${API_HOST}/experiments/${EXPERIMENT_ID}
echo "Эксперимент ${EXPERIMENT_ID} удален."

echo "\n--- ВСЕ ТЕСТЫ ПРОЙДЕНЫ УСПЕШНО ---"