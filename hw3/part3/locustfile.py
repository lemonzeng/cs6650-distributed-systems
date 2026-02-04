import time
from locust import HttpUser, task, between, FastHttpUser

# PART 1: Standard HttpUser (Uses Python requests)
# class StandardWebsiteUser(HttpUser):
#     wait_time = between(1, 2)

#     # 3:1 Ratio for GET vs POST
#     @task(3)
#     def get_albums(self):
#         # Changed from /items to /albums to match your Go Server
#         self.client.get("/albums", name="GET Albums")

#     @task(1)
#     def post_album(self):
#         # Changed from /items to /albums
#         # Using a JSON structure that matches a typical Album object
#         self.client.post("/albums", json={
#             "id": "99",
#             "title": "Locust Test Album",
#             "artist": "Molly",
#             "price": 19.99
#         }, name="POST Album")

# # PART 2: FastHttpUser (For Context Switching Experiment)
class FastWebsiteUser(FastHttpUser):
    wait_time = between(1, 2)

    @task(3)
    def get_albums(self):
        self.client.get("/albums", name="FAST GET Albums")

    @task(1)
    def post_album(self):
        self.client.post("/albums", json={
            "id": "100",
            "title": "Fast Locust Album",
            "artist": "Molly",
            "price": 29.99
        }, name="FAST POST Album")