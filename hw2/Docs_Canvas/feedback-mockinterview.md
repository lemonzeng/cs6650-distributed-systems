# Feedback
+ what's the difference among vitual envinroment, container, image and AMI?
+ Is Docker a container or image? which one is like a bluepoint?
+ what unique id define the instance? what if I create two instances in main.tf with one has been already created?

# Answers
### 1. What is the difference among virtual environment, container, image, and AMI?

These represent different levels of abstraction and isolation:

* **Virtual Environment (venv)**: Language-level isolation. It only isolates Python libraries. It shares the same Operating System and system files with your host.
* **Docker Image**: Application-level isolation. A read-only snapshot containing your code, libraries, and a minimal OS. It is the "package."
* **Docker Container**: The runtime instance. It is a "living" process created from an image. It is isolated from other containers but shares the host's OS kernel.
* **AWS AMI (Amazon Machine Image)**: OS-level isolation. A snapshot of an entire Virtual Machine, including the OS, Kernel, hardware configurations, and all pre-installed software.

### 2. Is Docker a container or image? Which one is like a blueprint?

**Docker** is the platform/engine that manages both.
* **The Blueprint = Docker Image**. It is static and contains all the instructions.
* **The Instance = Docker Container**. It is the active, running version of that blueprint.
* **Analogy**: An **Image** is like a recipe; a **Container** is the meal being cooked.

### 3. What unique ID defines the instance? What if I create two instances in main.tf with one has been already created?

* **Unique ID**: Every EC2 instance is defined by a unique **Instance ID** (e.g., `i-0a1b2c3d4e5f6g7h8`). Even if you delete an instance and create a new one with the same name, the Instance ID will change.
* **Terraform Logic**: Terraform is **Declarative**. It manages the "state" of your infrastructure.
    * If you have 1 instance managed by Terraform and you change the code to `count = 2`, Terraform compares your **code** (I want 2) with your **state file** (I have 1).
    * **Result**: It will only create **one additional instance**. It will not touch the existing one unless you changed a property that requires a full replacement (like the AMI ID).