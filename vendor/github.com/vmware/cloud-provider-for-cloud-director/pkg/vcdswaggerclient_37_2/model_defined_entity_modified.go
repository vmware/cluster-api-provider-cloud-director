/*
 * VMware Cloud Director OpenAPI
 *
 * VMware Cloud Director OpenAPI is a new API that is defined using the OpenAPI standards.<br/> This ReSTful API borrows some elements of the legacy VMware Cloud Director API and establishes new patterns for use as described below. <h4>Authentication</h4> Authentication and Authorization schemes are the same as those for the legacy APIs. You can authenticate using the JWT token via the <code>Authorization</code> header or specifying a session using <code>x-vcloud-authorization</code> (The latter form is deprecated). <h4>Operation Patterns</h4> This API follows the following general guidelines to establish a consistent CRUD pattern: <table> <tr>   <th>Operation</th><th>Description</th><th>Response Code</th><th>Response Content</th> </tr><tr>   <td>GET /items<td>Returns a paginated list of items<td>200<td>Response will include Navigational links to the items in the list. </tr><tr>   <td>POST /items<td>Returns newly created item<td>201<td>Content-Location header links to the newly created item </tr><tr>   <td>GET /items/urn<td>Returns an individual item<td>200<td>A single item using same data type as that included in list above </tr><tr>   <td>PUT /items/urn<td>Updates an individual item<td>200<td>Updated view of the item is returned </tr><tr>   <td>DELETE /items/urn<td>Deletes the item<td>204<td>No content is returned. </tr> </table> <h5>Asynchronous operations</h5> Asynchronous operations are determined by the server. In those cases, instead of responding as described above, the server responds with an HTTP Response code 202 and an empty body. The tracking task (which is the same task as all legacy API operations use) is linked via the URI provided in the <code>Location</code> header.<br/> All API calls can choose to service a request asynchronously or synchronously as determined by the server upon interpreting the request. Operations that choose to exhibit this dual behavior will have both options documented by specifying both response code(s) below. The caller must be prepared to handle responses to such API calls by inspecting the HTTP Response code. <h5>Error Conditions</h5> <b>All</b> operations report errors using the following error reporting rules: <ul>   <li>400: Bad Request - In event of bad request due to incorrect data or other user error</li>   <li>401: Bad Request - If user is unauthenticated or their session has expired</li>   <li>403: Forbidden - If the user is not authorized or the entity does not exist</li> </ul> <h4>OpenAPI Design Concepts and Principles</h4> <ul>   <li>IDs are full Uniform Resource Names (URNs).</li>   <li>OpenAPI's <code>Content-Type</code> is always <code>application/json</code></li>   <li>REST links are in the Link header.</li>   <ul>     <li>Multiple relationships for any link are represented by multiple values in a space-separated list.</li>     <li>Links have a custom VMware Cloud Director-specific &quot;model&quot; attribute that hints at the applicable data         type for the links.</li>     <li>title + rel + model attributes evaluates to a unique link.</li>     <li>Links follow Hypermedia as the Engine of Application State (HATEOAS) principles. Links are present if         certain operations are present and permitted for the user&quot;s current role and the state of the         referred entities.</li>   </ul>   <li>APIs follow a flat structure relying on cross-referencing other entities instead of the navigational style       used by the legacy VMware Cloud Director APIs.</li>   <li>Most endpoints that return a list support filtering and sorting similar to the query service in the legacy       VMware Cloud Director APIs.</li>   <li>Accept header must be included to specify the API version for the request similar to calls to existing legacy       VMware Cloud Director APIs.</li>   <li>Each feature has a version in the path element present in its URL.<br/>       <b>Note</b> API URL's without a version in their paths must be considered experimental.</li> </ul>
 *
 * API version: 37.2
 * Contact: https://code.vmware.com/support
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package swagger

// RDEState represents entity.state
type RDEState string

const (
	RDEStatePreCreated      RDEState = "PRE_CREATED"
	RDEStateResolved        RDEState = "RESOLVED"
	RDEStateResolutionError RDEState = "RESOLUTION_ERROR"
)

// Describes what a defined entity should look like.
type DefinedEntity struct {
	// The id of the defined entity in URN format.
	Id string `json:"id,omitempty"`
	// The URN ID of the defined entity type that the entity is an instance of. This is a read-only field.
	EntityType string `json:"entityType,omitempty"`
	// The name of the defined entity.
	Name string `json:"name"`
	// An external entity's id that this entity may have a relation to.
	ExternalId string `json:"externalId,omitempty"`
	// A JSON value representation. The JSON will be validated against the schema of the entityType that the entity is an instance of.
	Entity map[string]interface{} `json:"entity"`
	// Every entity is created in the \"PRE_CREATED\" state. Once an entity is ready to be validated against its schema, it will transition in another state - RESOLVED, if the entity is valid according to the schema, or RESOLUTION_ERROR otherwise. If an entity in an \"RESOLUTION_ERROR\" state is updated, it will transition to the initial \"PRE_CREATED\" state without performing any validation. If its in the \"RESOLVED\" state, then it will be validated against the entity type schema and throw an exception if its invalid.
	State RDEState `json:"state,omitempty"`
	// The \"entityState\" field may have the following values: \"PRE_CREATED\", \"RESOLVED\", \"RESOLUTION_ERROR\", \"IN_DELETION\". Every entity is created in the \"PRE_CREATED\" state. Once an entity is ready to be validated against its schema, it will transition in another state - \"RESOLVED\", if the entity is valid according to the schema, or \"RESOLUTION_ERROR\" otherwise. If an entity in an \"RESOLUTION_ERROR\" state is updated, it will transition to the initial \"PRE_CREATED\" state without performing any validation. If its in the \"RESOLVED\" state, then it will be validated against the entity type schema and throw an exception if its invalid. Once an entity is ready to be deleted, if any external resources need to be cleaned up, the entity deletion process can be set up into multiple stages. The entity will transition into state IN_DELETION when the finalization process starts and will be permanently deleted once it completes successfully. If the finalization is not successful, the entity will once it completes successfully. If the finalization is not successful, the entity will remain in IN_DELETION state.
	EntityState string `json:"entityState,omitempty"`
	// The owner of the defined entity.
	Owner *EntityReference `json:"owner,omitempty"`
	// The organization of the defined entity.
	Org *EntityReference `json:"org,omitempty"`
}
