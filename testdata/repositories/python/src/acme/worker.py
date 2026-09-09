"""Run the periodic worker independently with python -m acme.worker."""
from time import sleep

def process_pending_jobs():
    print("checking pending jobs")

def main():
    while True:
        process_pending_jobs()
        sleep(60)

if __name__ == "__main__":
    main()
