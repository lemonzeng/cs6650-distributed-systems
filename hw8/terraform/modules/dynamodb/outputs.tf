output "table_name" {
  value = aws_dynamodb_table.shopping_carts.name
}

output "table_arn" {
  value = aws_dynamodb_table.shopping_carts.arn
}
