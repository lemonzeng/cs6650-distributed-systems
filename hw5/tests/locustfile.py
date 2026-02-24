import random
from locust import task, between, HttpUser
from locust.contrib.fasthttp import FastHttpUser

class ProductBehavior:
    def on_start(self):
        """Initialize a random product ID for each user session to simulate interactions with different products."""
        self.product_id = random.randint(1, 10000)

    @task(1) # weight 1: imitate fewer writes (write)
    def post_product(self, client):
        client.post(f"/products/{self.product_id}/details", json={
            "name": f"Product-{self.product_id}",
            "description": "High-performance test item",
            "price": 99.9,
            "SKU": f"SKU-{self.product_id}",
            "Manufacturer": "NEU-Oakland-Lab"
        })

    @task(9) # weight 9: imitate more reads (read)
    def get_product(self, client):
        client.get(f"/products/{self.product_id}")

class NormalUser(HttpUser):
    wait_time = between(0.1, 0.5)
    
    @task
    def behavior(self):
        actions = ProductBehavior()
        actions.on_start()
        actions.get_product(self.client)
        actions.post_product(self.client)

class PowerUser(FastHttpUser):
    wait_time = between(0.1, 0.5)
    
    @task
    def behavior(self):
        actions = ProductBehavior()
        actions.on_start()
        actions.get_product(self.client)
        actions.post_product(self.client)