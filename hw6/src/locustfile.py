# locustfile.py — Load test for the product search service (Part II)
#
# TEST PLAN:
#   Test 1 — Baseline  : 5 users,  2 minutes  → expect ~60% CPU, fast responses
#   Test 2 — Break it  : 20 users, 3 minutes  → expect ~100% CPU, degraded responses
#
# HOW TO RUN (headless, no UI):
#   Test 1: locust --headless -u 5  -r 1 --run-time 2m --host http://<ECS-IP>:8080
#   Test 2: locust --headless -u 20 -r 2 --run-time 3m --host http://<ECS-IP>:8080
#
#   Or open the Locust web UI:
#   locust --host http://<ECS-IP>:8080
#   then visit http://localhost:8089

from locust import task, between
from locust.contrib.fasthttp import FastHttpUser

# Search terms that are guaranteed to hit the rotating category/brand pattern
SEARCH_TERMS = [
    # match every 10th product (100% match rate, large payloads)
    "Electronics",
    "Books",
    "Alpha",
    "Beta",
    "Home",
    "Sports",
    "Gamma",
    "Product",   
]


class SearchUser(FastHttpUser):
    # Simulates a single user continuously searching for products.

    # Short wait times to generate more load and better observe CPU saturation effects.
    wait_time = between(0.1, 0.5)

    # Index used to rotate through SEARCH_TERMS across tasks so each user
    # doesn't always query the same term, giving more realistic distribution.
    _term_index = 0

    @task(3)
    def search_by_category(self):
        """
        Search by category name (e.g. 'Electronics').
        Weight 3: categories produce many matches, generating more response
        payload and stressing serialization alongside CPU.
        """
        term = SEARCH_TERMS[self._term_index % len(SEARCH_TERMS)]
        self._term_index += 1
        self.client.get(
            f"/products/search?q={term}",
            # groups results in Locust UI
            name="/products/search?q=[category]", 
        )

    @task(1)
    def search_by_brand(self):
        """
        Search by brand name (e.g. 'Alpha').
        Weight 1: less frequent, simulates mixed traffic.
        """
        self.client.get(
            "/products/search?q=Alpha",
            name="/products/search?q=[brand]",
        )

    @task(1)
    def search_broad(self):
        """
        Search for 'Product', which matches every product name.
        Tests the path where totalFound is large but results are capped at 20.
        """
        self.client.get(
            "/products/search?q=Product",
            name="/products/search?q=[broad]",
        )
