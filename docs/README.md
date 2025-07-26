# A/B Testing System Documentation

## Services Overview

### Central API
- **Description**: Manages experiments and user decisions.
- **Endpoints**:
  - `/experiments`: Create, update, delete experiments.
  - `/decide`: Decide variant for a user.

### Example Sort App
- **Description**: Demonstrates A/B testing by sorting numbers differently based on experiment variants.
- **Endpoints**:
  - `/sort`: Accepts a JSON payload with a user ID and an array of numbers. Returns the sorted array based on the experiment variant.

## Running the System

### Prerequisites
- Docker and Docker Compose installed.

### Commands
1. **Build and Start Services**:
   ```bash
   docker-compose up --build
   ```

2. **Test Central API**:
   Use the cURL examples in `docs/curl-requests.md`.

3. **Test Example Sort App**:
   ```bash
   curl -X POST http://localhost:8081/sort \
     -H "Content-Type: application/json" \
     -d '{"userId": "user123", "numbers": [5, 2, 8, 1]}'
   ```

   Expected Response:
   ```json
   {
     "sortedNumbers": [1, 2, 5, 8] // or descending order based on variant
   }
   ```
