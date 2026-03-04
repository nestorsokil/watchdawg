import requests

METRICS_ENDPOINT = "http://watchdawg:9091"


def _fetch_value(url, name):
    response = requests.get(url)
    for line in response.text.split('\n'):
        if line.startswith(name):
            return float(line.split(' ')[1])
    return 0.0


class Prometheus:
    def __init__(self):
        self.url = f"{METRICS_ENDPOINT}/metrics"

    def counter(self, name):
        return PrometheusCounter(self, name)

    def gauge(self, name):
        return PrometheusGauge(self, name)


class PrometheusCounter:
    def __init__(self, prometheus, name):
        self.prometheus = prometheus
        self.name = name

    def __enter__(self):
        self.load()
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        pass

    def load(self):
        self.value = self._get_current()
        return self.value

    def has_increased(self, by=1):
        current = self._get_current()
        import logging
        logging.warning(f"current = {current} value = {self.value}")
        updated = current >= self.value + by
        if updated:
            self.value = current
        return updated

    def _get_current(self):
        return _fetch_value(self.prometheus.url, self.name)


class PrometheusGauge:
    def __init__(self, prometheus, name):
        self.prometheus = prometheus
        self.name = name

    def load(self):
        return _fetch_value(self.prometheus.url, self.name)
