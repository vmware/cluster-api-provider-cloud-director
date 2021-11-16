/*
 * VMware Cloud Director OpenAPI
 *
 * VMware Cloud Director OpenAPI is a new API that is defined using the OpenAPI standards.<br/> This ReSTful API borrows some elements of the legacy VMware Cloud Director API and establishes new patterns for use as described below. <h4>Authentication</h4> Authentication and Authorization schemes are the same as those for the legacy APIs. You can authenticate using the JWT token via the <code>Authorization</code> header or specifying a session using <code>x-vcloud-authorization</code> (The latter form is deprecated). <h4>Operation Patterns</h4> This API follows the following general guidelines to establish a consistent CRUD pattern: <table> <tr>   <th>Operation</th><th>Description</th><th>Response Code</th><th>Response Content</th> </tr><tr>   <td>GET /items<td>Returns a paginated list of items<td>200<td>Response will include Navigational links to the items in the list. </tr><tr>   <td>POST /items<td>Returns newly created item<td>201<td>Content-Location header links to the newly created item </tr><tr>   <td>GET /items/urn<td>Returns an individual item<td>200<td>A single item using same data type as that included in list above </tr><tr>   <td>PUT /items/urn<td>Updates an individual item<td>200<td>Updated view of the item is returned </tr><tr>   <td>DELETE /items/urn<td>Deletes the item<td>204<td>No content is returned. </tr> </table> <h5>Asynchronous operations</h5> Asynchronous operations are determined by the server. In those cases, instead of responding as described above, the server responds with an HTTP Response code 202 and an empty body. The tracking task (which is the same task as all legacy API operations use) is linked via the URI provided in the <code>Location</code> header.<br/> All API calls can choose to service a request asynchronously or synchronously as determined by the server upon interpreting the request. Operations that choose to exhibit this dual behavior will have both options documented by specifying both response code(s) below. The caller must be prepared to handle responses to such API calls by inspecting the HTTP Response code. <h5>Error Conditions</h5> <b>All</b> operations report errors using the following error reporting rules: <ul>   <li>400: Bad Request - In event of bad request due to incorrect data or other user error</li>   <li>401: Bad Request - If user is unauthenticated or their session has expired</li>   <li>403: Forbidden - If the user is not authorized or the entity does not exist</li> </ul> <h4>OpenAPI Design Concepts and Principles</h4> <ul>   <li>IDs are full Uniform Resource Names (URNs).</li>   <li>OpenAPI's <code>Content-Type</code> is always <code>application/json</code></li>   <li>REST links are in the Link header.</li>   <ul>     <li>Multiple relationships for any link are represented by multiple values in a space-separated list.</li>     <li>Links have a custom VMware Cloud Director-specific &quot;model&quot; attribute that hints at the applicable data         type for the links.</li>     <li>title + rel + model attributes evaluates to a unique link.</li>     <li>Links follow Hypermedia as the Engine of Application State (HATEOAS) principles. Links are present if         certain operations are present and permitted for the user&quot;s current role and the state of the         referred entities.</li>   </ul>   <li>APIs follow a flat structure relying on cross-referencing other entities instead of the navigational style       used by the legacy VMware Cloud Director APIs.</li>   <li>Most endpoints that return a list support filtering and sorting similar to the query service in the legacy       VMware Cloud Director APIs.</li>   <li>Accept header must be included to specify the API version for the request similar to calls to existing legacy       VMware Cloud Director APIs.</li>   <li>Each feature has a version in the path element present in its URL.<br/>       <b>Note</b> API URL's without a version in their paths must be considered experimental.</li> </ul>
 *
 * API version: 36.0
 * Contact: https://code.vmware.com/support
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package swagger

// Specifies the Load Balancer pool configuration.
type EdgeLoadBalancerPool struct {
	// Represents current status of the networking object.
	Status *NetworkingObjectStatusType `json:"status,omitempty"`
	// The unique ID of this Load Balancer Pool. On updates, the ID is required for the pool, while for create a new ID will be generated.
	Id string `json:"id,omitempty"`
	// The description of the Load Balancer Pool.
	Description string `json:"description,omitempty"`
	// True if Load Balancer Pool is enabled.
	Enabled bool `json:"enabled,omitempty"`
	// Whether passive monitoring for this pool is enabled or not.
	PassiveMonitoringEnabled bool `json:"passiveMonitoringEnabled,omitempty"`
	// The current health status of the pool. Possible values are: <ul> <li> UP - The pool is operational. <li> RUNNING - The pool is operational, but less than 50% of the pool members are up. <li> DOWN - All members in the pool are down. <li> DISABLED - Either the pool is disabled or all of the members are disabled. <li> UNAVAILABLE - The pool is unavailable. Examples: pool has no members or pool is not assigned to any virtual service. <li> UNKNOWN - The pool state is unknown. </ul>
	HealthStatus string `json:"healthStatus,omitempty"`
	// The total number of members in the pool.
	MemberCount int32 `json:"memberCount,omitempty"`
	// The number of enabled members in the pool.
	EnabledMemberCount int32 `json:"enabledMemberCount,omitempty"`
	// The number of enabled members in the pool that are operational.
	UpMemberCount int32 `json:"upMemberCount,omitempty"`
	// The localized message on the health of the pool.
	HealthMessage string `json:"healthMessage,omitempty"`
	// Name for the Load Balancer Pool. Name is unique across all pools for an Edge Gateway.
	Name string `json:"name"`
	// The destination server port used by the traffic sent to the member.
	DefaultPort int32 `json:"defaultPort,omitempty"`
	// Maximum time (in minutes) to gracefully disable a member. Virtual service waits for the specified time before terminating the existing connections to the members that are disabled. <code>Special values: 0 represents 'Immediate', -1 represents 'Infinite'.</code>
	GracefulTimeoutPeriod *int32 `json:"gracefulTimeoutPeriod,omitempty"`
	// The algorithm for choosing a member within the pool's list of available members for each new connection. Default value is \"LEAST_CONNECTIONS\". Supported algorithms are: <ul> <li>LEAST_CONNECTIONS <li>ROUND_ROBIN <li>CONSISTENT_HASH <li>FASTEST_RESPONSE <li>LEAST_LOAD <li>FEWEST_SERVERS <li>RANDOM <li>FEWEST_TASKS <li>CORE_AFFINITY </ul> <em>CONSISTENT_HASH</em> uses Source IP Address hash. Using <em>FASTEST_RESPONSE</em>, <em>LEAST_LOAD</em>, <em>FEWEST_SERVERS</em>, <em>RANDOM</em>, <em>FEWEST_TASKS</em>, <em>CORE_AFFINITY</em> algorithms may require additional licensing.
	Algorithm string `json:"algorithm,omitempty"`
	// Member server's health can be monitored by using one or more health monitors. Active monitors generate synthetic traffic and mark a server up or down based on the response.
	HealthMonitors []EdgeLoadBalancerHealthMonitor `json:"healthMonitors,omitempty"`
	// Selected persistence profile for the Load Balancer Pool.
	PersistenceProfile *EdgeLoadBalancerPersistenceProfile `json:"persistenceProfile,omitempty"`
	// The list of destination servers which are used by the Load Balancer Pool to direct load balanced traffic.
	Members []EdgeLoadBalancerPoolMember `json:"members,omitempty"`
	// The list of Load Balancer Virtual Services associated with this Load balancer Pool.
	VirtualServiceRefs []EntityReference `json:"virtualServiceRefs,omitempty"`
	// The Edge Gateway associated with this Load Balancer Pool.
	GatewayRef *EntityReference `json:"gatewayRef"`
	// The root certificates to use when validating certificates presented by the pool members.
	CaCertificateRefs []EntityReference `json:"caCertificateRefs,omitempty"`
	// Whether to check the common name of the certificate presented by the pool member. This cannot be enabled if no caCertificateRefs are specified.
	CommonNameCheckEnabled bool `json:"commonNameCheckEnabled,omitempty"`
	// A list of domain names which will be used to verify the common names or subject alternative names presented by the pool member certificates. It is performed only when common name check (commonNameCheckEnabled) is enabled. If common name check is enabled, but domain names are not specified then the incoming host header will be used to check the certificate.
	DomainNames []string `json:"domainNames,omitempty"`
}
