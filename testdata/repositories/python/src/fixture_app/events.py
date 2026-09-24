from kafka import KafkaConsumer

from .levels import fetch_level, retrieve_level


consumer = KafkaConsumer()


def handle_order(event):
    return retrieve_level(event["level_id"], fetch_level)


consumer.subscribe("orders.created", handle_order)


def subscribe_dynamic(runtime_consumer, topic, callback):
    runtime_consumer.subscribe(topic, callback)


def subscribe_direct():
    KafkaConsumer().subscribe("orders.direct", handle_order)


def bind_duplicate_callbacks():
    consumer.bind_pair(handle_order, handle_order)


def process_pending_jobs():
    print("checking pending jobs")


def run_worker():
    process_pending_jobs()  # startup: once before the loop
    while True:
        process_pending_jobs()


def process_once_per_item(items):
    for item in items:
        process_pending_jobs()


def register_callbacks(items):
    callbacks = []
    for item in items:
        def callback():
            process_pending_jobs()  # callback body has no enclosing execution loop
        callbacks.append(callback)
    return callbacks


async def consume_jobs(jobs):
    async for job in jobs:
        process_pending_jobs()


def record_order(event):
    return event


# Each call of a chain sits at its own attribute name, so both chained
# subscriptions keep their handlers and the two replaces stay two calls.
def subscribe_chained():
    KafkaConsumer().subscribe("orders.chained", handle_order).subscribe("orders.chained", record_order)


def chained_text_calls(path, name):
    normalized = "/".join(path.split("/"))
    repeated = name.replace("/", "-").replace("/", "-")
    head = path.split("/")[0].split("/")
    return [normalized, repeated, head]
