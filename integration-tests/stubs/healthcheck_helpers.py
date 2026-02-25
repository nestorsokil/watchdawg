class HealthcheckTarget:
    def __init__(self, module):
        self._module = module

    def fail_next(self, amount=1):
        self._module.fail_next(amount)

    def reset(self):
        self._module.reset()