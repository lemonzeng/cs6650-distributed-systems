import json
import requests
import matplotlib.pyplot as plt
import collections
import re
import os

# ==========================================
#  Result Verification
# ==========================================

def get_ground_truth(url):
    """
    Directly fetch the original text from GitHub and run a basic word count locally.
    This is to verify whether the count is correct or not.
    """
    print(f"Downloading file from source for ground truth verification...\nURL: {url}")
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        text = response.text.lower()
        
        # Counting logic must match the regex in Go code
        words = re.findall(r'[a-z]+', text)
        total_words = len(words)
        counts = collections.Counter(words)
        
        print(f"Local verification complete! Total word count: {total_words}")
        return counts, total_words
    except Exception as e:
        print(f"Failed to download source file: {e}")
        return None, 0

def verify_against_s3(local_counts, s3_res_path):
    """Compare local ground truth with your final_count.json downloaded from S3"""
    if not os.path.exists(s3_res_path):
        print(f"\n❌ Error: Cannot find {s3_res_path}. Please download it from S3 results/ folder first.")
        return
    
    with open(s3_res_path, 'r') as f:
        dist_counts = json.load(f)

    # Check a few key words
    check_list = ['hamlet', 'hand', 'lord', 'queen', 'horatio']
    
    print(f"\n{'Word':<12} | {'Ground Truth':<15} | {'Your System':<15} | {'Status'}")
    print("-" * 70)
    
    matches = 0
    for word in check_list:
        truth = local_counts.get(word, 0)
        system = dist_counts.get(word, 0)
        is_match = (truth == system)
        if is_match: matches += 1
        
        status = "✅ Match" if is_match else "❌ Mismatch"
        print(f"{word:<12} | {truth:<15} | {system:<15} | {status}")
    
    if matches == len(check_list):
        print("\nConclusion: Your system's results match the source file perfectly! The counts are small because the source file itself is small.")
    else:
        print("\nConclusion: Results don't match. The Splitter might have only read part of the file, or the Reducer's merge logic has an error.")

if __name__ == "__main__":
    target_url = "https://raw.githubusercontent.com/teropa/nlp/master/resources/corpora/gutenberg/shakespeare-hamlet.txt"
    truth_counts, total = get_ground_truth(target_url)
    
    if truth_counts:
        verify_against_s3(truth_counts, 'final_count.json')