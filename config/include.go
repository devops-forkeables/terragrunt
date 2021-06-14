package config

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Parse the config of the given include, if one is specified
func parseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), includePath)
	}

	return ParseConfigFile(includePath, terragruntOptions, includedConfig)
}

// handleInclude merges the included config into the current config depending on the merge strategy specified by the
// user.
func handleInclude(
	config *TerragruntConfig,
	terragruntInclude *terragruntInclude,
	terragruntOptions *options.TerragruntOptions,
) (*TerragruntConfig, error) {
	mergeStrategy, err := terragruntInclude.Include.GetMergeStrategy()
	if err != nil {
		return config, err
	}

	switch mergeStrategy {
	case NoMerge:
		terragruntOptions.Logger.Debugf("Included config %s has strategy no merge: not merging config in.", terragruntInclude.Include.Path)
		return config, nil
	case ShallowMerge:
		terragruntOptions.Logger.Debugf("Included config %s has strategy shallow merge: merging config in (shallow).", terragruntInclude.Include.Path)
		includedConfig, err := parseIncludedConfig(terragruntInclude.Include, terragruntOptions)
		if err != nil {
			return nil, err
		}
		return mergeConfigWithIncludedConfig(config, includedConfig, terragruntOptions)
	case DeepMerge:
		terragruntOptions.Logger.Debugf("Included config %s has strategy deep merge: merging config in (deep).", terragruntInclude.Include.Path)
		terragruntOptions.Logger.Error("Deep merge is not implemented yet")
		return nil, errors.WithStackTrace(fmt.Errorf("Not implemented"))
	}

	return nil, errors.WithStackTrace(fmt.Errorf("Impossible condition"))
}

// Merge the given config with an included config. Anything specified in the current config will override the contents
// of the included config. If the included config is nil, just return the current config.
func mergeConfigWithIncludedConfig(config *TerragruntConfig, includedConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if config.RemoteState != nil {
		includedConfig.RemoteState = config.RemoteState
	}

	if config.PreventDestroy != nil {
		includedConfig.PreventDestroy = config.PreventDestroy
	}

	// Skip has to be set specifically in each file that should be skipped
	includedConfig.Skip = config.Skip

	if config.Terraform != nil {
		if includedConfig.Terraform == nil {
			includedConfig.Terraform = config.Terraform
		} else {
			if config.Terraform.Source != nil {
				includedConfig.Terraform.Source = config.Terraform.Source
			}
			mergeExtraArgs(terragruntOptions, config.Terraform.ExtraArgs, &includedConfig.Terraform.ExtraArgs)

			mergeHooks(terragruntOptions, config.Terraform.BeforeHooks, &includedConfig.Terraform.BeforeHooks)
			mergeHooks(terragruntOptions, config.Terraform.AfterHooks, &includedConfig.Terraform.AfterHooks)
		}
	}

	if config.Dependencies != nil {
		includedConfig.Dependencies = config.Dependencies
	}

	if config.DownloadDir != "" {
		includedConfig.DownloadDir = config.DownloadDir
	}

	if config.IamRole != "" {
		includedConfig.IamRole = config.IamRole
	}

	if config.IamAssumeRoleDuration != nil {
		includedConfig.IamAssumeRoleDuration = config.IamAssumeRoleDuration
	}

	if config.TerraformVersionConstraint != "" {
		includedConfig.TerraformVersionConstraint = config.TerraformVersionConstraint
	}

	if config.TerraformBinary != "" {
		includedConfig.TerraformBinary = config.TerraformBinary
	}

	if config.RetryableErrors != nil {
		includedConfig.RetryableErrors = config.RetryableErrors
	}

	if config.RetryMaxAttempts != nil {
		includedConfig.RetryMaxAttempts = config.RetryMaxAttempts
	}

	if config.RetrySleepIntervalSec != nil {
		includedConfig.RetrySleepIntervalSec = config.RetrySleepIntervalSec
	}

	if config.TerragruntVersionConstraint != "" {
		includedConfig.TerragruntVersionConstraint = config.TerragruntVersionConstraint
	}

	// Merge the generate configs. This is a shallow merge. Meaning, if the child has the same name generate block, then the
	// child's generate block will override the parent's block.
	for key, val := range config.GenerateConfigs {
		includedConfig.GenerateConfigs[key] = val
	}

	if config.Inputs != nil {
		includedConfig.Inputs = mergeInputs(config.Inputs, includedConfig.Inputs)
	}

	return includedConfig, nil
}

// Merge the extra arguments.
//
// If a child's extra_arguments has the same name a parent's extra_arguments,
// then the child's extra_arguments will be selected (and the parent's ignored)
// If a child's extra_arguments has a different name from all of the parent's extra_arguments,
// then the child's extra_arguments will be added to the end  of the parents.
// Therefore, terragrunt will put the child extra_arguments after the parent's
// extra_arguments on the terraform cli.
// Therefore, if .tfvar files from both the parent and child contain a variable
// with the same name, the value from the child will win.
func mergeExtraArgs(terragruntOptions *options.TerragruntOptions, childExtraArgs []TerraformExtraArguments, parentExtraArgs *[]TerraformExtraArguments) {
	result := *parentExtraArgs
	for _, child := range childExtraArgs {
		parentExtraArgsWithSameName := getIndexOfExtraArgsWithName(result, child.Name)
		if parentExtraArgsWithSameName != -1 {
			// If the parent contains an extra_arguments with the same name as the child,
			// then override the parent's extra_arguments with the child's.
			terragruntOptions.Logger.Debugf("extra_arguments '%v' from child overriding parent", child.Name)
			result[parentExtraArgsWithSameName] = child
		} else {
			// If the parent does not contain an extra_arguments with the same name as the child
			// then add the child to the end.
			// This ensures the child extra_arguments are added to the command line after the parent extra_arguments.
			result = append(result, child)
		}
	}
	*parentExtraArgs = result
}

func mergeInputs(childInputs map[string]interface{}, parentInputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}

	for key, value := range parentInputs {
		out[key] = value
	}

	for key, value := range childInputs {
		out[key] = value
	}

	return out
}

// Merge the hooks (before_hook and after_hook).
//
// If a child's hook (before_hook or after_hook) has the same name a parent's hook,
// then the child's hook will be selected (and the parent's ignored)
// If a child's hook has a different name from all of the parent's hooks,
// then the child's hook will be added to the end of the parent's.
// Therefore, the child with the same name overrides the parent
func mergeHooks(terragruntOptions *options.TerragruntOptions, childHooks []Hook, parentHooks *[]Hook) {
	result := *parentHooks
	for _, child := range childHooks {
		parentHookWithSameName := getIndexOfHookWithName(result, child.Name)
		if parentHookWithSameName != -1 {
			// If the parent contains a hook with the same name as the child,
			// then override the parent's hook with the child's.
			terragruntOptions.Logger.Debugf("hook '%v' from child overriding parent", child.Name)
			result[parentHookWithSameName] = child
		} else {
			// If the parent does not contain a hook with the same name as the child
			// then add the child to the end.
			result = append(result, child)
		}
	}
	*parentHooks = result
}
