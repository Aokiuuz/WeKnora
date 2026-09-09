"""No-network regression tests for cumulative model-spending reservations."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('acceptance',Path(__file__).with_name('evaluation-openrouter-acceptance.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

class Response:
    status=200
    def __init__(self,data): self.data=data
    def read(self): return json.dumps(self.data).encode()

class Opener:
    def __init__(self,data): self.data,self.calls=data,0
    def open(self,*args,**kwargs):
        self.calls+=1
        if isinstance(self.data,Exception): raise self.data
        return Response(self.data)

class BudgetTest(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.root=Path(self.tmp.name)
        self.relay=module.BudgetRelay(0,'test-placeholder',self.root,self.root/'budget.json')
        self.payload={'model':module.CHAT,'messages':[{'role':'user','content':'fixed test'}],'max_tokens':16}
    def tearDown(self):
        self.relay.server.server_close();self.relay.budget_guard.close();self.tmp.cleanup()
    def test_known_zero_is_settled(self):
        self.relay.opener=Opener({'usage':{'cost':0}})
        self.relay.forward('/v1/chat/completions',self.payload)
        record=json.loads((self.root/'budget.json').read_text())['requests'][0]
        self.assertEqual(record['actual_usd'],'0')
    def test_unknown_transport_keeps_full_reservation(self):
        self.relay.opener=Opener(TimeoutError('fixture timeout'))
        with self.assertRaises(TimeoutError): self.relay.forward('/v1/chat/completions',self.payload)
        record=json.loads((self.root/'budget.json').read_text())['requests'][0]
        self.assertGreater(float(record['reserved_usd']),0)
        self.assertNotIn('actual_usd',record)
    def test_budget_rejects_before_network(self):
        self.relay.budget['requests']=[{'reserved_usd':'19.999'}]
        self.relay.opener=Opener({})
        with self.assertRaisesRegex(AssertionError,'budget exhausted'):
            self.relay.forward('/v1/chat/completions',self.payload)
        self.assertEqual(self.relay.opener.calls,0)
    def test_independent_budget_handle_cannot_take_lock(self):
        import fcntl
        with (self.root/'budget.lock').open('a') as lock:
            with self.assertRaises(BlockingIOError): fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)

if __name__=='__main__': unittest.main()
