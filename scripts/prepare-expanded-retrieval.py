"""Freeze 50 answerable questions against 500 real passages per dataset."""
import argparse
import hashlib
import json
from pathlib import Path


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data', type=Path, required=True)
    args = parser.parse_args()
    cases = json.loads((args.data / 'holdout.json').read_text(encoding='utf-8'))
    for dataset, source in [('cmrc', 'cmrc2018-dev.json'), ('squad', 'squad2-dev.json')]:
        questions = [q for q in cases if q['dataset'] == dataset and q['type'] == 'answerable'][:50]
        passages = {digest(c): {'pid': digest(c), 'content': c,
                    'metadata': {'title': t, 'source_dataset': dataset, 'license': 'CC-BY-SA-4.0'}}
                    for q in questions for t, c in q['contexts']}
        original = json.loads((args.data / 'sources' / source).read_text(encoding='utf-8'))
        extra = [(a['title'], p['context']) for a in original['data'] for p in a['paragraphs']]
        extra.sort(key=lambda p: digest('20260911-retrieval-500' + p[1]))
        for title, content in extra:
            if len(passages) >= 500:
                break
            passages.setdefault(digest(content), {'pid': digest(content), 'content': content,
                'metadata': {'title': title, 'source_dataset': dataset, 'license': 'CC-BY-SA-4.0'}})
        assert len(passages) == 500 and len(questions) == 50
        registry = {'passages': list(passages.values()),
                    'questions': [{'qid': q['qid'], 'question': q['question'], 'answer': q['answers'][0]} for q in questions],
                    'relevance': [{'qid': q['qid'], 'pid': digest(content), 'grade': 1} for q in questions for _, content in q['contexts']]}
        raw = json.dumps(registry, ensure_ascii=False, indent=2)
        (args.data / (dataset + '-retrieval-500.json')).write_text(raw, encoding='utf-8', newline='\n')
        print(json.dumps({'dataset': dataset, 'questions': len(questions), 'passages': len(passages), 'sha256': digest(raw)}))


if __name__ == '__main__':
    main()
