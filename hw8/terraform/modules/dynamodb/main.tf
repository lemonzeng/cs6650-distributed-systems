# DynamoDB single table for shopping carts.
# Design: partition key = cart_id (String).
# Items list is embedded as a DynamoDB List attribute — no sort key needed
# since all access patterns operate on a full cart by cart_id.
# PAY_PER_REQUEST billing avoids capacity planning for the assignment.

resource "aws_dynamodb_table" "shopping_carts" {
  name         = var.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "S"
  }

  tags = {
    Name = var.table_name
  }
}
