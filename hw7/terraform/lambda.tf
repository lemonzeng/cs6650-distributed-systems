# ---------------------------------------------------------------------------
# Part III — Lambda function subscribed directly to SNS
# Demonstrates serverless alternative to SQS + ECS workers
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${var.project_name}-order-processor"
  retention_in_days = 7
}

resource "aws_lambda_function" "order_processor" {
  filename         = "../lambda/lambda.zip"
  function_name    = "${var.project_name}-order-processor"
  role             = data.aws_iam_role.lab_role.arn
  handler          = "bootstrap"
  runtime          = "provided.al2"
  memory_size      = 512
  timeout          = 10  # must exceed 3s payment processing time

  source_code_hash = filebase64sha256("../lambda/lambda.zip")

  depends_on = [aws_cloudwatch_log_group.lambda]
}

# Allow SNS to invoke the Lambda function
resource "aws_lambda_permission" "sns_invoke" {
  statement_id  = "AllowSNSInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.order_processor.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.orders.arn
}

# Subscribe Lambda directly to the SNS topic (no SQS needed)
resource "aws_sns_topic_subscription" "lambda" {
  topic_arn = aws_sns_topic.orders.arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.order_processor.arn
}
