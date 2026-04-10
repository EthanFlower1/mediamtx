# KAI-282 Face Recognition Base Model — Vendor RFP Scope

**Author:** lead-ai · **Date:** 2026-04-08 · **Status:** DRAFT — pending CTO budget approval
**Timeline:** 2-week sprint from approval date, targeting vendor selection by 2026-05-15
**Gate:** CTO budget authority (§14 of KAI-282 memo)

---

## 1. Background

Per lead-security's APPROVE-WITH-CHANGES on the KAI-282 architecture memo (2026-04-08), **Path B (pre-audited vendor model)** is the primary path for the face recognition base model. Options 1-5 (off-the-shelf academic weights with scraped-dataset provenance) were killed due to GDPR Art. 10 data governance and licence issues.

This document scopes the vendor evaluation sprint.

## 2. Vendor shortlist

| Vendor | Headquarters | Key product | NIST FRVT ranking | EU presence | Notes |
|--------|-------------|-------------|-------------------|-------------|-------|
| **Paravision** | San Francisco, USA | Face SDK | Top-10 FRVT 1:N | US-only currently | Strong NIST results, good API |
| **Idemia** | Courbevoie, France | ID3 Face | Top-5 FRVT 1:N | HQ in EU | Government-grade, likely expensive |
| **Innovatrics** | Bratislava, Slovakia | SmartFace, DOT | Top-10 FRVT 1:N | HQ in EU | Mid-market, good pricing history |
| **Corsight** | Tel Aviv, Israel | Facial Intelligence | Top-20 FRVT 1:N | EU subsidiary | Edge-focused, may align with on-prem |

Additional vendors may be added during discovery. The evaluation matrix (§5) is the decision framework regardless of vendor count.

## 3. RFP requirements (non-negotiable)

Every vendor MUST deliver the following as a condition of advancing past initial screening:

### 3.1 EU AI Act Annex IV evidence package

| Requirement | Article | Deliverable |
|-------------|---------|-------------|
| Training data description | Art. 10(2)(a-g) | Dataset card: source, size, demographic composition, collection methodology, consent/licence chain |
| Per-demographic fairness audit | Art. 10(5), Art. 15 | TPR/FPR per Fitzpatrick I-II/III-IV/V-VI × age 18-30/31-50/51+ × gender M/F/other (18 buckets) |
| Equalised-odds gap report | Art. 10(5) | Max gap across buckets; Kaivue threshold: ≤0.05 absolute |
| Presentation attack detection results | Art. 15 | iBeta PAD Level 1 or equivalent third-party PAD testing |
| Model architecture documentation | Annex IV §2(a) | Architecture, backbone, loss function, training procedure, hyperparameters |
| Version history and change log | Annex IV §2(a) | Full version history with accuracy metrics per version |
| Data retention and deletion procedures | Art. 10(2)(f) | How vendor handles training data post-delivery (deletion, anonymisation) |

### 3.2 Licensing and integration

| Requirement | Detail |
|-------------|--------|
| Commercial licence for embedding in Kaivue product | Must allow redistribution to Kaivue's end customers (deployers) |
| ONNX export support | Kaivue's inference stack (KAI-277 Triton + KAI-278 ONNX Runtime) requires ONNX format |
| Embedding dimensionality | 512-dim strongly preferred (matches KAI-292 pgvector schema); 256 or 1024 acceptable with schema migration |
| On-prem deployment support | Model must run fully on-prem without internet callback (Seam #5: fail-open recording) |
| Model update cadence | Vendor must commit to at least annual model updates with Annex IV evidence refresh |
| No mandatory cloud dependency | Vendor SDK must not require vendor cloud for inference (licensing server acceptable if air-gap-compatible) |

### 3.3 Technical benchmarks (Kaivue-side evaluation)

During the 2-week sprint, each vendor's model will be evaluated on Kaivue's internal test infrastructure:

| Benchmark | Metric | Threshold |
|-----------|--------|-----------|
| NIST FRVT 1:N rank | FNMR@FMR=1e-4 | Top-20 |
| Kaivue fairness test set | Equalised-odds gap | ≤0.05 |
| Kaivue PAD test set (Tier 3 in-house) | APCER/BPCER | Vendor must pass; thresholds TBD with lead-security |
| Inference latency (A10G GPU) | P99 per face | ≤15ms |
| Inference latency (CPU, on-prem) | P99 per face | ≤100ms |
| Embedding extraction throughput | Faces/second (A10G) | ≥200 |
| ONNX model size | Disk footprint | ≤500MB |
| Memory footprint (inference) | Peak RSS | ≤2GB |

### 3.4 Contractual requirements

| Requirement | Detail |
|-------------|--------|
| Annex IV evidence delivery SLA | Vendor delivers evidence package within 30 days of contract signing |
| Indemnification for training data provenance | Vendor indemnifies Kaivue against claims related to training data copyright/consent |
| Notification of model accuracy regression | Vendor must notify Kaivue within 5 business days of discovering accuracy regression in deployed version |
| Right to audit | Kaivue (or Kaivue's auditor) may audit vendor's Art. 10 data governance procedures annually |
| Source code escrow | Model weights + training pipeline escrowed with a neutral third party in case of vendor insolvency |
| EU data residency option | Training data and model artifacts must be storable in EU region if required by customer DPIA |

## 4. Sprint structure (2 weeks)

### Week 1: Vendor outreach and initial screening

| Day | Activity |
|-----|----------|
| 1-2 | Send RFP to all 4 vendors; schedule 60-min intro calls |
| 3-4 | Intro calls: present requirements, confirm vendor can deliver §3.1-3.4 |
| 5 | Initial screening decision: advance vendors who confirm all non-negotiables |

### Week 2: Technical evaluation

| Day | Activity |
|-----|----------|
| 6-7 | Receive evaluation SDK/weights from advancing vendors; run §3.3 benchmarks |
| 8-9 | Review Annex IV evidence packages; lead-security reviews fairness audits |
| 10 | Scoring meeting: lead-ai + lead-security + CTO. Final vendor selection. |

### Post-sprint (week 3)

- Legal review of vendor contract
- CTO signs off on pricing
- Integration PoC begins (1-week spike: ONNX export → KAI-277 Triton → pgvector embedding)

## 5. Evaluation matrix (scoring rubric)

| Category | Weight | Scoring |
|----------|--------|---------|
| Annex IV evidence completeness | 25% | 0-10: completeness of §3.1 deliverables |
| Fairness metrics | 20% | 0-10: equalised-odds gap, per-bucket TPR coverage |
| Technical benchmarks (§3.3) | 20% | 0-10: latency, throughput, accuracy vs. thresholds |
| Licensing flexibility | 15% | 0-10: on-prem support, no cloud dependency, redistribution rights |
| Pricing | 10% | 0-10: annual cost relative to budget |
| Vendor stability and support | 10% | 0-10: company size, support SLA, EU presence, escrow willingness |

Minimum passing score: 6.0 weighted average. If no vendor passes, fall back to Path A (licensed dataset fine-tune).

## 6. Budget estimate

| Item | Estimate | Notes |
|------|----------|-------|
| Vendor evaluation licences (4 vendors × $0-5k) | $0-20k | Some vendors provide free eval; others charge |
| Staff time (lead-ai + lead-security, 2 weeks) | Internal | Opportunity cost only |
| Annual model licence (selected vendor) | $50k-200k | Wide range; depends on per-seat vs. per-deployment pricing |
| Integration PoC (1 week) | Internal | Post-selection, before contract |
| Legal review of vendor contract | $5-10k | External counsel if needed |

**Total initial outlay:** $55k-230k (first year). CTO budget authority required.

## 7. Fallback: Path A (licensed dataset fine-tune)

If no vendor meets requirements or pricing exceeds budget:

1. License a commercial face dataset (e.g., DigiFace-1M synthetic, or negotiate VoxCeleb2 commercial licence)
2. Fine-tune ArcFace/AdaFace backbone on licensed data
3. Run full Annex IV evidence generation in-house (adds 3-6 months)
4. Additional cost: dataset licence ($10-50k) + compute ($5-20k) + fairness testing ($15-30k)

Path A timeline: 4-8 months vs. Path B's 1-2 months. This is why Path B is primary.

---

*End of document. CTO: please confirm budget authority so lead-ai can begin vendor outreach.*
