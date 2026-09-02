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
