# For local development with self-signed certificates
export INSI_API_KEY="your-api-key"
export INSI_IS_LOCAL=true
export INSI_SKIP_VERIFY=true
cd examples/insi
go run .

# For production (with valid certificates)
export INSI_API_KEY="your-api-key"
# Don't set INSI_SKIP_VERIFY or set it to false
cd examples/insi
go run .