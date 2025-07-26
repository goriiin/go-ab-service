# File: docs/curl-requests.md
# cURL-запросы для взаимодействия с central-api

API_HOST="http://localhost:8080"
EXPERIMENT_ID=""

# 1. Создать эксперимент для теста сортировки
# Создается неактивный (DRAFT) эксперимент с двумя вариантами (A и B)
# и правилом таргетинга: участвуют только пользователи с атрибутом "use_sort_test" = true
curl -s -X POST ${API_HOST}/experiments \
-H "Content-Type: application/json" \
-d '{
"layer_id": "sorting_layer",
"targeting_rules": [
{ "attribute": "use_sort_test", "operator": "EQUALS", "value": true }
],
"variants": [
{ "name": "variant-a-asc", "bucket_range": [0, 499] },
{ "name": "variant-b-desc", "bucket_range": [500, 999] }
]
}' > response.json

EXPERIMENT_ID=$(cat response.json | jq -r .id)
echo "Created Experiment ID: ${EXPERIMENT_ID}"

# 2. Активировать эксперимент (изменить статус на ACTIVE)
# Для этого используется тот же JSON, что и для создания, но с измененным полем status
curl -i -X PUT ${API_HOST}/experiments/${EXPERIMENT_ID} \
-H "Content-Type: application/json" \
-d '{
"layer_id": "sorting_layer",
"status": "ACTIVE",
"targeting_rules": [
{ "attribute": "use_sort_test", "operator": "EQUALS", "value": true }
],
"variants": [
{ "name": "variant-a-asc", "bucket_range": [0, 499] },
{ "name": "variant-b-desc", "bucket_range": [500, 999] }
]
}'

# 3. Запросить решение для пользователя, который подпадает под критерии
curl -i -X POST ${API_HOST}/decide \
-H "Content-Type: application/json" \
-d '{
"user_id": "user-100",
"attributes": { "use_sort_test": true }
}'

 Ожидаемый ответ содержит {"EXPERIMENT_ID": "variant-a-asc"} или {"EXPERIMENT_ID": "variant-b-desc"}

# 4. Запросить решение для пользователя, который НЕ подпадает под критерии
curl -i -X POST ${API_HOST}/decide \
-H "Content-Type: application/json" \
-d '{
"user_id": "user-200",
"attributes": { "use_sort_test": false }
}'

Ожидаемый ответ: {}

# 5. Удалить эксперимент
curl -i -X DELETE ${API_HOST}/experiments/${EXPERIMENT_ID}