"""Real HTTP evaluation over larger frozen corpora, using the shared USD guard."""
import argparse
import importlib.util
import json
from pathlib import Path
import time

spec = importlib.util.spec_from_file_location('acceptance', Path(__file__).with_name('evaluation-openrouter-acceptance.py'))
module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)


class Expanded(module.Acceptance):
    def __init__(self, args, key):
        super().__init__(args, key)
        self.env.update(CONCURRENCY_POOL_SIZE='4', BATCH_EMBED_SIZE='5')
        self.embedding_concurrency = 4
        self.manifest['concurrency_pool_size'] = 4
        self.manifest['embedding_batch_size'] = 5

    def execute(self):
        self.supplier.start()
        try:
            self.start(1); self.register()
            for dataset in ('cmrc', 'squad'):
                fixture = self.dataset(dataset + ': 50 questions / 500 passages', self.args.data / (dataset + '-retrieval-500.json'))
                for model, label in [(module.CHAT, 'deepseek'), (module.COMPARE, 'kimi')]:
                    for attempt in range(3):
                        try:
                            self.run_live(dataset + '-' + label + (f'-retry{attempt}' if attempt else ''), fixture, model)
                            break
                        except AssertionError as error:
                            if not any(code in str(error) for code in ('status code: 429', 'status code: 502', 'status code: 503')) or attempt == 2:
                                raise
                            time.sleep(10)
            self.manifest['status'] = 'passed'
            self.manifest['label_scope'] = 'Original question context relevance; answerable questions only. Other semantically relevant passages may exist in the corpus.'
            self.manifest['human_review_status'] = 'excluded by user'
        finally:
            self.stop(); self.supplier.close()
            self.manifest['supplier_errors'] = self.supplier.errors
            self.manifest['paid_provider_requests'] = len(self.supplier.records)
            module.save(self.output / 'manifest.json', self.manifest)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--data', type=Path, required=True)
    parser.add_argument('--budget-file', type=Path, required=True)
    parser.add_argument('--server-binary', type=Path, required=True)
    parser.add_argument('--port', type=int, default=18808)
    parser.add_argument('--supplier-port', type=int, default=18810)
    args = parser.parse_args(); credentials = json.loads(input())
    assert credentials['approved_usd'] == 20
    Expanded(args, credentials['openrouter_key']).execute()
