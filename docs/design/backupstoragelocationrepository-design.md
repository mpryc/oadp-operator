# BackupStorageLocationRepository (BSLR) Design

## Abstract

This document proposes the BackupStorageLocationRepository (BSLR), a new Custom Resource for the OADP operator designed to manage multiple Kopia repositories within a single shared BackupStorageLocation (BSL). Rather than replacing existing functionality, the BSLR augments the standard Velero workflow. It provides an OADP-operated management layer for creating and configuring additional Kopia repositories, which coexist with the primary repository managed by Velero's core controllers. This design enables advanced use cases like workload isolation and independent credential management for Kopia, and does not impact other Velero-supported storage backends.

## Background

The current architecture of OADP tightly couples each BackupStorageLocation (BSL) with a single Kopia repository scoped to the BSL and namespace. This repository is provisioned and controlled entirely by Velero’s core components. While this setup is adequate for standard backup scenarios, it introduces significant limitations when more flexible or granular configurations are needed.

A key limitation is the inability to create and manage multiple independent repositories under the same BSL. This becomes particularly problematic in environments that require workload isolation or the use of distinct credentials across different repositories. As a result, users cannot tailor backup configurations to match the needs of specific workloads.

One common example is KubeVirt virtual machines (VMs): OpenShift administrators currently have no way to assign a dedicated repository to a single VM. This is a limitation especially important in cases where backups are scoped to individual user data (e.g. home directory) rather than the entire cluster. Furthermore, isolated repositories are essential in zero-trust scenarios, where VM owners must be able to secure their data with passwords unknown to cluster administrators. The current model also lacks support for customizing repository-level settings, such as encryption or compression policies.

In summary, OADP does not currently offer a mechanism to manage multiple, independent Kopia repositories within a shared storage location. The proposed BackupStorageLocationRepository (BSLR) feature is designed to close this gap and provide the flexibility required by advanced and security-conscious use cases.

## Scope

This design applies **only** to the `kopia` repository type supported by Velero. Other repository types (such as restic) are out of scope and not supported by the BSLR component. All features and behavior described herein assume the use of Kopia as the underlying repository technology.

## Goals

* **Support Multiple Repository Instances per BSL in the same namespace**
  Enable the creation and management of multiple Kopia repository instances that share a single BackupStorageLocation (BSL) within the same namespace.

* **Enable BSL-Agnostic Repository Access**
  Allow access to Kopia repositories independently of Velero's backup and restore mechanisms, enabling use cases outside the traditional OADP/Velero flow.

* **Integrate with OADP Operator**
  Seamlessly integrate the BSLR controller with the OADP operator, leveraging controller-runtime best practices for reconciliation, status tracking, and error handling.

* **Ensure Compatibility with Existing BSLs**
  Maintain compatibility with current BackupStorageLocation objects and their configurations, avoiding disruptions to existing OADP DPA setups.

* **Lay Groundwork for Future Enhancements**
  Establish the foundation for future enhancements, including the creation of a Backup Storage Location Server (BSLS). This server will act as a managed service for OpenShift users - especially those using KubeVirt virtual machines - enabling them to easily back up and restore user data by connecting to the BSLS.

## Non-Goals

* **BSLS Design**: This design document does not cover the Backup Storage Location Server (BSLS) design, which is documented separately.

* **Velero Backup and Restore Workflow Replacement**: This design does not aim to replace or modify the existing Velero backup and restore mechanisms. Instead, it complements them by enabling alternative workflows that operate alongside traditional Velero operations.

* **New Backup Storage Backend Support**: The BSLR does not introduce support for new types of backup storage backends. It relies on existing BSL-compatible storage configurations and does not extend the supported storage types.

* **User-Facing Backup Interfaces**: This design does not provide user-facing interfaces, CLIs, or web consoles for managing backups or repositories. It focuses on the underlying infrastructure and API-level operations.

* **Metrics and Observability Integrations**: While observability is an important consideration for production systems, this design does not currently include Prometheus metrics integration, alerting features, or comprehensive monitoring capabilities.


## High-Level Design

The BSLR will use the BSLR controller within OADP to manage the lifecycle of backup storage repositories. It will interface with the Velero repository manager to prepare and maintain Kopia repositories.

## Detailed Design

### BSLR Overview

* **Pointer to Repository**: Each BSLR serves as a reference to a single Kopia repository within a BSL.
* **Credential Management**: BSL credentials are stored as OpenShift secrets referenced by the BSL. Kopia-specific credentials are used by the BSLR controller and can also be stored as OpenShift secrets.
* **Kopia Configuration**: BSLR may contain Kopia-specific configuration parameters such as encryption algorithms, compression settings, and other repository-specific options that are not part of the standard BSL configuration.

### BSLR Controller Responsibilities

* **BSL Monitoring**: The controller observes BSLs and ensures a BSLR exists per BSL.
* **Kopia Repository Management**: For BSLRs not directly tied to a BSL, the controller creates and manages Kopia repositories.

### Controller Architecture

The controller follows standard Kubernetes controller patterns and includes:

* **Reconciler**: `BackupStorageLocationRepositoryReconciler`
* **RBAC**: Requires full permissions on `backupstoragelocationrepositories`, including status and finalizers.

### Reconciliation Flow

This controller manages BackupStorageLocationRepository (BSLR) objects and ensures that their associated Kopia repositories are correctly initialized and kept in sync with the desired state.

It reacts to changes in either BackupStorageLocation (BSL) or BSLR objects.

#### Reconcile on BSL Changes

_Note: The defult BSLR spec is marked as managed by the OADP controller._

1. When a `BackupStorageLocation` (BSL) changes:
   - Check if a corresponding default `BackupStorageLocationRepository` (BSLR) exists for the BSL.
   - If a default BSLR exists:
     - Ensure it is not marked for deletion.
     - Determine if the BSLR spec needs to be updated based on changes in the BSL.
     - If an update is required, update the BSLR spec.
   - If it does not exists:
     - Create default BSLR.
   - Exit reconciliation.

#### Reconcile on BSLR Changes

_Note: Default BSLRs are managed via BSL changes reconciliation. This flow only applies to non-default BSLRs._

1. If the BSLR is the default for its associated `BackupStorageLocation` (BSL):  
   - Exit reconciliation (handled by BSL watcher).

2. Retrieve the associated BSL.  
3. Validate:  
   - BSL is **not being deleted**.  
   - BSL is in the `Available` phase.  
   - BSLR is **not marked for deletion**.

4. Proceed to ensure the Kopia repository exists and matches the BSLR spec:  
   - If missing, initialize repository.  
   - If spec differs, update repository.

5. Update BSLR status to reflect current state.  
6. Exit reconciliation.


## Alternatives Considered

One alternative considered was using prefixes defined within the `BackupStorageLocation` (BSL) specification to support multiple repositories.  
However, this approach delays the creation of the Kopia repository until a backup is actually initiated. As a result, it prevents the controller from provisioning and initializing the Kopia server in advance — a key requirement for repository access and configuration before a Velero backup can run.

## Security Considerations

All interactions with BSLs and repositories follow security best practices to ensure data protection.

## Compatibility

BSLR is designed for seamless integration with Velero and existing storage solutions used by the OADP operator.

## Open Issues

None at this time.
